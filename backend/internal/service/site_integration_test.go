package service_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/provisioner"
	"github.com/nazxf/opencloud/backend/internal/queue"
	"github.com/nazxf/opencloud/backend/internal/repository"
	"github.com/nazxf/opencloud/backend/internal/service"
)

func openPhase2TestDB(t *testing.T) *bun.DB {
	t.Helper()
	db := openTestDB(t)
	var count int
	err := db.NewRaw(`
		SELECT count(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name IN ('nodes', 'sites', 'jobs')`).Scan(context.Background(), &count)
	require.NoError(t, err)
	if count != 3 {
		t.Skip("Phase 2 tables missing; run migrations first")
	}
	return db
}

type phase2Fixture struct {
	db       *bun.DB
	account  *model.Account
	nodes    []*model.Node
	sites    *repository.SiteRepo
	nodeRepo *repository.NodeRepo
	jobs     *repository.JobRepo
	audit    *repository.AuditRepo
	svc      *service.SiteService
}

func newPhase2Fixture(t *testing.T, capacities ...int) *phase2Fixture {
	t.Helper()
	db := openPhase2TestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	account := &model.Account{
		ID:        uuid.New(),
		Name:      "Phase 2 " + uuid.NewString(),
		Status:    model.AccountActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := db.NewInsert().Model(account).Exec(ctx)
	require.NoError(t, err)

	nodeRepo := repository.NewNodeRepo(db)
	nodes := make([]*model.Node, 0, len(capacities))
	for i, capacity := range capacities {
		node, err := nodeRepo.Create(
			ctx,
			fmt.Sprintf("phase2-%d-%s.invalid", i, uuid.NewString()),
			string(provisioner.BackendFake),
			capacity,
			nil,
		)
		require.NoError(t, err)
		nodes = append(nodes, node)
	}
	sites := repository.NewSiteRepo(db)
	jobs := repository.NewJobRepo(db)
	audit := repository.NewAuditRepo(db)
	fixture := &phase2Fixture{
		db:       db,
		account:  account,
		nodes:    nodes,
		sites:    sites,
		nodeRepo: nodeRepo,
		jobs:     jobs,
		audit:    audit,
		svc: service.NewSiteService(
			db,
			sites,
			nodeRepo,
			jobs,
			audit,
			string(provisioner.BackendFake),
			"opencloud/site-static:phase2",
		),
	}
	t.Cleanup(func() {
		_, _ = db.NewDelete().Model((*model.Job)(nil)).Where("account_id = ?", account.ID).Exec(ctx)
		_, _ = db.NewDelete().Model((*model.Site)(nil)).Where("account_id = ?", account.ID).Exec(ctx)
		for _, node := range nodes {
			_, _ = db.NewDelete().Model((*model.Node)(nil)).Where("id = ?", node.ID).Exec(ctx)
		}
		_, _ = db.NewDelete().Model((*model.Account)(nil)).Where("id = ?", account.ID).Exec(ctx)
	})
	return fixture
}

func TestConcurrentSiteCreateBalancesCapacityAndClaimsJobsOnce(t *testing.T) {
	fx := newPhase2Fixture(t, 4, 4)
	ctx := context.Background()

	const callers = 8
	results := make(chan *model.Site, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			site, err := fx.svc.Create(
				ctx,
				"phase2-actor",
				fx.account.ID,
				fmt.Sprintf("create-%d-%s", index, uuid.NewString()),
				service.CreateSiteRequest{Domain: fmt.Sprintf("site-%d-%s.example.test", index, uuid.NewString())},
			)
			results <- site
			errs <- err
		}(i)
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	siteIDs := make(map[uuid.UUID]struct{}, callers)
	for site := range results {
		require.NotNil(t, site)
		siteIDs[site.ID] = struct{}{}
	}
	require.Len(t, siteIDs, callers)

	for _, nodeID := range []uuid.UUID{fx.nodes[0].ID, fx.nodes[1].ID} {
		node, err := fx.nodeRepo.Get(ctx, nodeID)
		require.NoError(t, err)
		require.Equal(t, 4, node.UsedSites)
	}

	queued, err := fx.db.NewSelect().
		Model((*model.Job)(nil)).
		Where("account_id = ?", fx.account.ID).
		Where("status = ?", model.JobQueued).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, callers, queued)

	claimed := make(chan uuid.UUID, callers)
	claimErrs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			job, err := fx.jobs.Claim(ctx, fmt.Sprintf("claim-worker-%d", index))
			if err == nil {
				claimed <- job.ID
			}
			claimErrs <- err
		}(i)
	}
	wg.Wait()
	close(claimed)
	close(claimErrs)
	for err := range claimErrs {
		require.NoError(t, err)
	}
	claimedIDs := make(map[uuid.UUID]struct{}, callers)
	for id := range claimed {
		claimedIDs[id] = struct{}{}
	}
	require.Len(t, claimedIDs, callers, "SKIP LOCKED must not double-claim a job")

	_, err = fx.svc.Create(
		ctx,
		"phase2-actor",
		fx.account.ID,
		uuid.NewString(),
		service.CreateSiteRequest{Domain: "over-capacity-" + uuid.NewString() + ".example.test"},
	)
	require.Error(t, err)
	require.Equal(t, "CONFLICT", apperr.As(err).Code)
}

func TestConcurrentIdempotentCreateReturnsOneWinner(t *testing.T) {
	fx := newPhase2Fixture(t, 1)
	ctx := context.Background()
	key := "same-request-" + uuid.NewString()
	domain := "idempotent-" + uuid.NewString() + ".example.test"

	results := make(chan *model.Site, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			site, err := fx.svc.Create(
				ctx,
				"phase2-actor",
				fx.account.ID,
				key,
				service.CreateSiteRequest{Domain: domain},
			)
			results <- site
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var winner uuid.UUID
	for site := range results {
		require.NotNil(t, site)
		if winner == uuid.Nil {
			winner = site.ID
		}
		require.Equal(t, winner, site.ID)
	}
	node, err := fx.nodeRepo.Get(ctx, fx.nodes[0].ID)
	require.NoError(t, err)
	require.Equal(t, 1, node.UsedSites)
	count, err := fx.db.NewSelect().Model((*model.Site)(nil)).Where("account_id = ?", fx.account.ID).Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestSiteCreateAuditFailureRollsBackResourceJobAndCapacity(t *testing.T) {
	fx := newPhase2Fixture(t, 1)
	ctx := context.Background()
	_, err := fx.db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION reject_phase2_site_audit() RETURNS trigger AS $$
		BEGIN
			IF NEW.action = 'site.create.queued' THEN
				RAISE EXCEPTION 'forced phase2 audit failure';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER reject_phase2_site_audit
			BEFORE INSERT ON audit_logs
			FOR EACH ROW EXECUTE FUNCTION reject_phase2_site_audit();
	`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = fx.db.ExecContext(ctx, `
			DROP TRIGGER IF EXISTS reject_phase2_site_audit ON audit_logs;
			DROP FUNCTION IF EXISTS reject_phase2_site_audit();
		`)
	})

	_, err = fx.svc.Create(
		ctx,
		"phase2-actor",
		fx.account.ID,
		uuid.NewString(),
		service.CreateSiteRequest{Domain: "audit-fail-" + uuid.NewString() + ".example.test"},
	)
	require.Error(t, err)

	siteCount, countErr := fx.db.NewSelect().Model((*model.Site)(nil)).Where("account_id = ?", fx.account.ID).Count(ctx)
	require.NoError(t, countErr)
	require.Zero(t, siteCount)
	jobCount, countErr := fx.db.NewSelect().Model((*model.Job)(nil)).Where("account_id = ?", fx.account.ID).Count(ctx)
	require.NoError(t, countErr)
	require.Zero(t, jobCount)
	node, getErr := fx.nodeRepo.Get(ctx, fx.nodes[0].ID)
	require.NoError(t, getErr)
	require.Zero(t, node.UsedSites)
}

func TestSiteLifecycleThroughDurableJobsReleasesCapacityExactlyOnce(t *testing.T) {
	fx := newPhase2Fixture(t, 1)
	ctx := context.Background()
	fake := provisioner.NewFake()
	processor := queue.NewProcessor(fx.db, fx.sites, fx.nodeRepo, fx.jobs, fx.audit, fake, nil, nil, nil)

	site, err := fx.svc.Create(
		ctx,
		"phase2-actor",
		fx.account.ID,
		uuid.NewString(),
		service.CreateSiteRequest{Domain: "lifecycle-" + uuid.NewString() + ".example.test"},
	)
	require.NoError(t, err)
	processNextJob(ctx, t, fx.jobs, processor, model.JobProvisionSite)
	site, err = fx.svc.Get(ctx, fx.account.ID, site.ID)
	require.NoError(t, err)
	require.Equal(t, model.SiteActive, site.Status)

	_, err = fx.svc.Suspend(ctx, "phase2-actor", fx.account.ID, site.ID)
	require.NoError(t, err)
	processNextJob(ctx, t, fx.jobs, processor, model.JobSuspendSite)
	site, err = fx.svc.Get(ctx, fx.account.ID, site.ID)
	require.NoError(t, err)
	require.Equal(t, model.SiteSuspended, site.Status)

	_, err = fx.svc.Resume(ctx, "phase2-actor", fx.account.ID, site.ID)
	require.NoError(t, err)
	processNextJob(ctx, t, fx.jobs, processor, model.JobResumeSite)
	site, err = fx.svc.Get(ctx, fx.account.ID, site.ID)
	require.NoError(t, err)
	require.Equal(t, model.SiteActive, site.Status)

	_, err = fx.svc.Delete(ctx, "phase2-actor", fx.account.ID, site.ID)
	require.NoError(t, err)
	processNextJob(ctx, t, fx.jobs, processor, model.JobDeleteSite)

	_, err = fx.svc.Get(ctx, fx.account.ID, site.ID)
	require.Error(t, err)
	require.Equal(t, "NOT_FOUND", apperr.As(err).Code)
	node, err := fx.nodeRepo.Get(ctx, fx.nodes[0].ID)
	require.NoError(t, err)
	require.Zero(t, node.UsedSites)

	repeated, err := fx.svc.Delete(ctx, "phase2-actor", fx.account.ID, site.ID)
	require.NoError(t, err)
	require.Equal(t, model.SiteDeleted, repeated.Status)
	deleteJobs, err := fx.db.NewSelect().
		Model((*model.Job)(nil)).
		Where("account_id = ?", fx.account.ID).
		Where("kind = ?", model.JobDeleteSite).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, deleteJobs, "repeated delete must not enqueue another job")

	ref := provisioner.SiteRef{SiteID: site.ID, AccountID: site.AccountID, NodeID: site.NodeID}
	require.NoError(t, fake.DeleteSite(ctx, ref))
	node, err = fx.nodeRepo.Get(ctx, fx.nodes[0].ID)
	require.NoError(t, err)
	require.Zero(t, node.UsedSites)
}

func TestDeleteIntentWinsAgainstInFlightProvisionCompletion(t *testing.T) {
	fx := newPhase2Fixture(t, 1)
	ctx := context.Background()
	fake := provisioner.NewFake()
	processor := queue.NewProcessor(fx.db, fx.sites, fx.nodeRepo, fx.jobs, fx.audit, fake, nil, nil, nil)

	site, err := fx.svc.Create(
		ctx,
		"phase2-actor",
		fx.account.ID,
		uuid.NewString(),
		service.CreateSiteRequest{Domain: "delete-race-" + uuid.NewString() + ".example.test"},
	)
	require.NoError(t, err)
	_, err = fx.svc.Delete(ctx, "phase2-actor", fx.account.ID, site.ID)
	require.NoError(t, err)

	processNextJob(ctx, t, fx.jobs, processor, model.JobProvisionSite)
	current, err := fx.sites.GetForWorker(ctx, site.ID)
	require.NoError(t, err)
	require.Equal(t, model.SiteDeleting, current.Status, "older provisioning must not overwrite delete intent")

	processNextJob(ctx, t, fx.jobs, processor, model.JobDeleteSite)
	current, err = fx.sites.GetForWorker(ctx, site.ID)
	require.NoError(t, err)
	require.Equal(t, model.SiteDeleted, current.Status)
}

func processNextJob(
	ctx context.Context,
	t *testing.T,
	jobs *repository.JobRepo,
	processor *queue.Processor,
	wantKind string,
) {
	t.Helper()
	workerID := "lifecycle-worker"
	job, err := jobs.Claim(ctx, workerID)
	require.NoError(t, err)
	require.Equal(t, wantKind, job.Kind)
	require.NoError(t, processor.Handle(ctx, job, workerID))
}
