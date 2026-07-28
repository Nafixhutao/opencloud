package service_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/credential"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/provisioner"
	"github.com/nazxf/opencloud/backend/internal/queue"
	"github.com/nazxf/opencloud/backend/internal/repository"
	"github.com/nazxf/opencloud/backend/internal/service"
)

type managedDatabaseFixture struct {
	db        *bun.DB
	account   *model.Account
	rows      *repository.ManagedDatabaseRepo
	jobs      *repository.JobRepo
	audit     *repository.AuditRepo
	svc       *service.ManagedDatabaseService
	fake      *provisioner.FakeDatabase
	cipher    *credential.Cipher
	processor *queue.Processor
}

func newManagedDatabaseFixture(t *testing.T) *managedDatabaseFixture {
	t.Helper()
	db := openPhase2TestDB(t)
	ctx := context.Background()
	var tableCount int
	err := db.NewRaw(`
		SELECT count(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name IN ('databases', 'database_credentials')`).Scan(ctx, &tableCount)
	require.NoError(t, err)
	if tableCount != 2 {
		t.Skip("customer database tables missing; run migrations first")
	}

	now := time.Now().UTC()
	account := &model.Account{
		ID:        uuid.New(),
		Name:      "Database fixture " + uuid.NewString(),
		Status:    model.AccountActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = db.NewInsert().Model(account).Exec(ctx)
	require.NoError(t, err)

	key := make([]byte, 32)
	_, err = rand.Read(key)
	require.NoError(t, err)
	cipher, err := credential.New(base64.StdEncoding.EncodeToString(key))
	require.NoError(t, err)

	rows := repository.NewManagedDatabaseRepo(db)
	jobs := repository.NewJobRepo(db)
	audit := repository.NewAuditRepo(db)
	fake := provisioner.NewFakeDatabase()
	svc := service.NewManagedDatabaseService(db, rows, jobs, audit, true, cipher)
	processor := queue.NewProcessor(
		db,
		repository.NewSiteRepo(db),
		repository.NewNodeRepo(db),
		jobs,
		audit,
		provisioner.NewFake(),
		rows,
		fake,
		cipher,
	)
	fixture := &managedDatabaseFixture{
		db:        db,
		account:   account,
		rows:      rows,
		jobs:      jobs,
		audit:     audit,
		svc:       svc,
		fake:      fake,
		cipher:    cipher,
		processor: processor,
	}
	t.Cleanup(func() {
		_, _ = db.NewDelete().Model((*model.Job)(nil)).Where("account_id = ?", account.ID).Exec(ctx)
		_, _ = db.NewDelete().
			Model((*model.ManagedDatabase)(nil)).
			Where("account_id = ?", account.ID).
			Exec(ctx)
		_, _ = db.NewDelete().Model((*model.Account)(nil)).Where("id = ?", account.ID).Exec(ctx)
	})
	return fixture
}

func TestConcurrentManagedDatabaseCreateReturnsOneWinner(t *testing.T) {
	fx := newManagedDatabaseFixture(t)
	ctx := context.Background()
	key := "database-create-" + uuid.NewString()

	const callers = 6
	results := make(chan *model.ManagedDatabase, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			row, err := fx.svc.Create(
				ctx,
				"database-actor",
				fx.account.ID,
				key,
				service.CreateDatabaseRequest{Name: "primary_db", Engine: model.DatabaseEnginePostgres},
			)
			results <- row
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
	for row := range results {
		require.NotNil(t, row)
		if winner == uuid.Nil {
			winner = row.ID
		}
		require.Equal(t, winner, row.ID)
	}
	count, err := fx.db.NewSelect().
		Model((*model.ManagedDatabase)(nil)).
		Where("account_id = ?", fx.account.ID).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	jobCount, err := fx.db.NewSelect().
		Model((*model.Job)(nil)).
		Where("account_id = ?", fx.account.ID).
		Where("kind = ?", model.JobProvisionDatabase).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, jobCount)
}

func TestManagedDatabaseLifecycleAndConcurrentCredentialReveal(t *testing.T) {
	fx := newManagedDatabaseFixture(t)
	ctx := context.Background()
	row, err := fx.svc.Create(
		ctx,
		"database-actor",
		fx.account.ID,
		uuid.NewString(),
		service.CreateDatabaseRequest{Name: "orders", Engine: model.DatabaseEngineMariaDB},
	)
	require.NoError(t, err)

	job := processManagedDatabaseJob(ctx, t, fx.jobs, fx.processor, model.JobProvisionDatabase)
	require.JSONEq(t, fmt.Sprintf(`{"database_id":%q}`, row.ID), string(job.Payload))
	require.NotContains(t, string(job.Payload), "password")

	current, err := fx.svc.Get(ctx, fx.account.ID, row.ID)
	require.NoError(t, err)
	require.Equal(t, model.DatabaseActive, current.Status)
	require.True(t, current.CredentialAvailable)

	type revealResult struct {
		credentials *provisioner.DatabaseCredentials
		err         error
	}
	reveals := make(chan revealResult, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			credentials, err := fx.svc.RevealCredential(
				ctx,
				"database-actor",
				fx.account.ID,
				row.ID,
			)
			reveals <- revealResult{credentials: credentials, err: err}
		}()
	}
	wg.Wait()
	close(reveals)

	var successes, conflicts int
	for reveal := range reveals {
		if reveal.err == nil {
			successes++
			require.NotNil(t, reveal.credentials)
			require.Equal(t, model.DatabaseEngineMariaDB, reveal.credentials.Engine)
			require.NotEmpty(t, reveal.credentials.Password)
			require.True(t, reveal.credentials.TLSRequired)
			continue
		}
		require.Equal(t, "CONFLICT", apperr.As(reveal.err).Code)
		conflicts++
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
	current, err = fx.svc.Get(ctx, fx.account.ID, row.ID)
	require.NoError(t, err)
	require.False(t, current.CredentialAvailable)

	queuedDelete, err := fx.svc.Delete(ctx, "database-actor", fx.account.ID, row.ID)
	require.NoError(t, err)
	require.Equal(t, model.DatabaseDeleting, queuedDelete.Status)
	processManagedDatabaseJob(ctx, t, fx.jobs, fx.processor, model.JobDeleteDatabase)
	require.False(t, fx.fake.Exists(row.ID))

	_, err = fx.svc.Get(ctx, fx.account.ID, row.ID)
	require.Error(t, err)
	require.Equal(t, "NOT_FOUND", apperr.As(err).Code)
	repeated, err := fx.svc.Delete(ctx, "database-actor", fx.account.ID, row.ID)
	require.NoError(t, err)
	require.Equal(t, model.DatabaseDeleted, repeated.Status)
	deleteJobCount, err := fx.db.NewSelect().
		Model((*model.Job)(nil)).
		Where("account_id = ?", fx.account.ID).
		Where("kind = ?", model.JobDeleteDatabase).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, deleteJobCount)
}

func TestManagedDatabaseCreateAuditFailureRollsBackIntentAndJob(t *testing.T) {
	fx := newManagedDatabaseFixture(t)
	ctx := context.Background()
	installAuditFailureTrigger(t, fx.db, model.AuditDatabaseCreateQueued)

	_, err := fx.svc.Create(
		ctx,
		"database-actor",
		fx.account.ID,
		uuid.NewString(),
		service.CreateDatabaseRequest{Name: "audit_fail", Engine: model.DatabaseEnginePostgres},
	)
	require.Error(t, err)
	rowCount, countErr := fx.db.NewSelect().
		Model((*model.ManagedDatabase)(nil)).
		Where("account_id = ?", fx.account.ID).
		Count(ctx)
	require.NoError(t, countErr)
	require.Zero(t, rowCount)
	jobCount, countErr := fx.db.NewSelect().
		Model((*model.Job)(nil)).
		Where("account_id = ?", fx.account.ID).
		Count(ctx)
	require.NoError(t, countErr)
	require.Zero(t, jobCount)
}

func TestManagedDatabaseProvisionAndRevealRequireDurableAudit(t *testing.T) {
	fx := newManagedDatabaseFixture(t)
	ctx := context.Background()
	row, err := fx.svc.Create(
		ctx,
		"database-actor",
		fx.account.ID,
		uuid.NewString(),
		service.CreateDatabaseRequest{Name: "durable_audit", Engine: model.DatabaseEnginePostgres},
	)
	require.NoError(t, err)
	job, err := fx.jobs.Claim(ctx, "database-audit-worker")
	require.NoError(t, err)
	require.Equal(t, model.JobProvisionDatabase, job.Kind)

	removeProvisionTrigger := installAuditFailureTriggerManual(
		t,
		fx.db,
		model.AuditDatabaseProvisioned,
	)
	err = fx.processor.Handle(ctx, job, "database-audit-worker")
	require.Error(t, err)
	current, getErr := fx.rows.GetForWorker(ctx, row.ID)
	require.NoError(t, getErr)
	require.Equal(t, model.DatabaseProvisioning, current.Status)
	available, getErr := fx.rows.CredentialExists(ctx, row.ID)
	require.NoError(t, getErr)
	require.False(t, available)

	removeProvisionTrigger()
	require.NoError(t, fx.processor.Handle(ctx, job, "database-audit-worker"))
	current, getErr = fx.svc.Get(ctx, fx.account.ID, row.ID)
	require.NoError(t, getErr)
	require.Equal(t, model.DatabaseActive, current.Status)
	require.True(t, current.CredentialAvailable)

	removeRevealTrigger := installAuditFailureTriggerManual(
		t,
		fx.db,
		model.AuditDatabaseCredentialRevealed,
	)
	_, err = fx.svc.RevealCredential(ctx, "database-actor", fx.account.ID, row.ID)
	require.Error(t, err)
	available, getErr = fx.rows.CredentialExists(ctx, row.ID)
	require.NoError(t, getErr)
	require.True(t, available, "failed audit must roll back one-time credential consumption")

	removeRevealTrigger()
	credentials, err := fx.svc.RevealCredential(ctx, "database-actor", fx.account.ID, row.ID)
	require.NoError(t, err)
	require.NotEmpty(t, credentials.Password)
}

func TestManagedDatabaseDeleteIntentWinsInFlightProvision(t *testing.T) {
	fx := newManagedDatabaseFixture(t)
	ctx := context.Background()
	row, err := fx.svc.Create(
		ctx,
		"database-actor",
		fx.account.ID,
		uuid.NewString(),
		service.CreateDatabaseRequest{Name: "delete_race", Engine: model.DatabaseEnginePostgres},
	)
	require.NoError(t, err)
	_, err = fx.svc.Delete(ctx, "database-actor", fx.account.ID, row.ID)
	require.NoError(t, err)

	processManagedDatabaseJob(ctx, t, fx.jobs, fx.processor, model.JobProvisionDatabase)
	current, err := fx.rows.GetForWorker(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, model.DatabaseDeleting, current.Status)
	available, err := fx.rows.CredentialExists(ctx, row.ID)
	require.NoError(t, err)
	require.False(t, available)
	require.False(t, fx.fake.Exists(row.ID), "superseded job must not create a resource")

	processManagedDatabaseJob(ctx, t, fx.jobs, fx.processor, model.JobDeleteDatabase)
	current, err = fx.rows.GetForWorker(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, model.DatabaseDeleted, current.Status)
	require.False(t, fx.fake.Exists(row.ID))
}

func TestConcurrentManagedDatabaseDeleteSerializesAheadOfClaimedProvision(t *testing.T) {
	fx := newManagedDatabaseFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	row, err := fx.svc.Create(
		ctx,
		"database-actor",
		fx.account.ID,
		uuid.NewString(),
		service.CreateDatabaseRequest{Name: "delete_first", Engine: model.DatabaseEnginePostgres},
	)
	require.NoError(t, err)
	provisionJob, err := fx.jobs.Claim(ctx, "database-provision-worker")
	require.NoError(t, err)
	require.Equal(t, model.JobProvisionDatabase, provisionJob.Kind)

	_, err = fx.svc.Delete(ctx, "database-actor", fx.account.ID, row.ID)
	require.NoError(t, err)
	deleteJob, err := fx.jobs.Claim(ctx, "database-delete-worker")
	require.NoError(t, err)
	require.Equal(t, model.JobDeleteDatabase, deleteJob.Kind)

	controlled := newDeleteFirstDatabaseProvisioner(fx.fake)
	processor := fx.processorWith(controlled)
	deleteResult := make(chan error, 1)
	go func() {
		deleteResult <- processor.Handle(ctx, deleteJob, "database-delete-worker")
	}()
	requireChannelClosed(ctx, t, controlled.deleteStarted)

	provisionInvoked := make(chan struct{})
	provisionResult := make(chan error, 1)
	go func() {
		close(provisionInvoked)
		provisionResult <- processor.Handle(ctx, provisionJob, "database-provision-worker")
	}()
	requireChannelClosed(ctx, t, provisionInvoked)
	close(controlled.releaseDelete)

	require.NoError(t, requireChannelValue(ctx, t, deleteResult))
	require.NoError(t, requireChannelValue(ctx, t, provisionResult))
	require.Zero(t, controlled.createCalls.Load())
	require.False(t, fx.fake.Exists(row.ID))

	current, err := fx.rows.GetForWorker(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, model.DatabaseDeleted, current.Status)
	available, err := fx.rows.CredentialExists(ctx, row.ID)
	require.NoError(t, err)
	require.False(t, available)
	activeJobs, err := fx.db.NewSelect().
		Model((*model.Job)(nil)).
		Where("account_id = ?", fx.account.ID).
		Where("status IN (?, ?)", model.JobQueued, model.JobRunning).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, activeJobs)
}

func TestSupersededManagedDatabaseCleanupFailureDoesNotCompleteProvision(t *testing.T) {
	fx := newManagedDatabaseFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	row, err := fx.svc.Create(
		ctx,
		"database-actor",
		fx.account.ID,
		uuid.NewString(),
		service.CreateDatabaseRequest{Name: "cleanup_retry", Engine: model.DatabaseEnginePostgres},
	)
	require.NoError(t, err)
	provisionJob, err := fx.jobs.Claim(ctx, "database-cleanup-worker")
	require.NoError(t, err)
	require.Equal(t, model.JobProvisionDatabase, provisionJob.Kind)

	controlled := newBlockedCreateDatabaseProvisioner(fx.fake)
	processor := fx.processorWith(controlled)
	provisionResult := make(chan error, 1)
	go func() {
		provisionResult <- processor.Handle(ctx, provisionJob, "database-cleanup-worker")
	}()
	requireChannelClosed(ctx, t, controlled.createFinished)

	_, err = fx.svc.Delete(ctx, "database-actor", fx.account.ID, row.ID)
	require.NoError(t, err)
	close(controlled.releaseCreate)

	err = requireChannelValue(ctx, t, provisionResult)
	require.Error(t, err)
	require.ErrorContains(t, err, "remove superseded provisioned database")
	require.True(t, fx.fake.Exists(row.ID))

	storedJob := new(model.Job)
	require.NoError(t, fx.db.NewSelect().Model(storedJob).Where("id = ?", provisionJob.ID).Scan(ctx))
	require.Equal(t, model.JobRunning, storedJob.Status)
	available, err := fx.rows.CredentialExists(ctx, row.ID)
	require.NoError(t, err)
	require.False(t, available)

	require.NoError(
		t,
		fx.jobs.Retry(
			ctx,
			provisionJob.ID,
			"database-cleanup-worker",
			"provisioner operation failed",
			time.Now().UTC(),
		),
	)
	controlled.failDelete.Store(false)
	processManagedDatabaseJob(ctx, t, fx.jobs, processor, model.JobDeleteDatabase)
	processManagedDatabaseJob(ctx, t, fx.jobs, processor, model.JobProvisionDatabase)
	require.False(t, fx.fake.Exists(row.ID))
}

func TestManagedDatabaseTenantScopeHidesCrossAccountRows(t *testing.T) {
	fx := newManagedDatabaseFixture(t)
	ctx := context.Background()
	row, err := fx.svc.Create(
		ctx,
		"database-actor",
		fx.account.ID,
		uuid.NewString(),
		service.CreateDatabaseRequest{Name: "tenant_scope", Engine: model.DatabaseEnginePostgres},
	)
	require.NoError(t, err)

	_, err = fx.svc.Get(ctx, uuid.New(), row.ID)
	require.Error(t, err)
	require.Equal(t, "NOT_FOUND", apperr.As(err).Code)
	_, err = fx.svc.Delete(ctx, "other-actor", uuid.New(), row.ID)
	require.Error(t, err)
	require.Equal(t, "NOT_FOUND", apperr.As(err).Code)
}

func (f *managedDatabaseFixture) processorWith(
	databaseProvisioner provisioner.DatabaseProvisioner,
) *queue.Processor {
	return queue.NewProcessor(
		f.db,
		repository.NewSiteRepo(f.db),
		repository.NewNodeRepo(f.db),
		f.jobs,
		f.audit,
		provisioner.NewFake(),
		f.rows,
		databaseProvisioner,
		f.cipher,
	)
}

type deleteFirstDatabaseProvisioner struct {
	delegate      *provisioner.FakeDatabase
	deleteStarted chan struct{}
	releaseDelete chan struct{}
	createCalls   atomic.Int32
}

func newDeleteFirstDatabaseProvisioner(
	delegate *provisioner.FakeDatabase,
) *deleteFirstDatabaseProvisioner {
	return &deleteFirstDatabaseProvisioner{
		delegate:      delegate,
		deleteStarted: make(chan struct{}),
		releaseDelete: make(chan struct{}),
	}
}

func (p *deleteFirstDatabaseProvisioner) CreateDatabase(
	ctx context.Context,
	spec provisioner.DatabaseSpec,
) (provisioner.DatabaseCredentials, error) {
	p.createCalls.Add(1)
	return p.delegate.CreateDatabase(ctx, spec)
}

func (p *deleteFirstDatabaseProvisioner) DeleteDatabase(
	ctx context.Context,
	ref provisioner.DatabaseRef,
) error {
	close(p.deleteStarted)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.releaseDelete:
		return p.delegate.DeleteDatabase(ctx, ref)
	}
}

type blockedCreateDatabaseProvisioner struct {
	delegate       *provisioner.FakeDatabase
	createFinished chan struct{}
	releaseCreate  chan struct{}
	failDelete     atomic.Bool
}

func newBlockedCreateDatabaseProvisioner(
	delegate *provisioner.FakeDatabase,
) *blockedCreateDatabaseProvisioner {
	provider := &blockedCreateDatabaseProvisioner{
		delegate:       delegate,
		createFinished: make(chan struct{}),
		releaseCreate:  make(chan struct{}),
	}
	provider.failDelete.Store(true)
	return provider
}

func (p *blockedCreateDatabaseProvisioner) CreateDatabase(
	ctx context.Context,
	spec provisioner.DatabaseSpec,
) (provisioner.DatabaseCredentials, error) {
	credentials, err := p.delegate.CreateDatabase(ctx, spec)
	if err != nil {
		return provisioner.DatabaseCredentials{}, err
	}
	close(p.createFinished)
	select {
	case <-ctx.Done():
		return provisioner.DatabaseCredentials{}, ctx.Err()
	case <-p.releaseCreate:
		return credentials, nil
	}
}

func (p *blockedCreateDatabaseProvisioner) DeleteDatabase(
	ctx context.Context,
	ref provisioner.DatabaseRef,
) error {
	if p.failDelete.Load() {
		return errors.New("forced compensating cleanup failure")
	}
	return p.delegate.DeleteDatabase(ctx, ref)
}

func requireChannelClosed(ctx context.Context, t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-ch:
	}
}

func requireChannelValue[T any](ctx context.Context, t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
		var zero T
		return zero
	case value := <-ch:
		return value
	}
}

func processManagedDatabaseJob(
	ctx context.Context,
	t *testing.T,
	jobs *repository.JobRepo,
	processor *queue.Processor,
	wantKind string,
) *model.Job {
	t.Helper()
	workerID := "managed-database-worker"
	job, err := jobs.Claim(ctx, workerID)
	require.NoError(t, err)
	require.Equal(t, wantKind, job.Kind)
	require.NoError(t, processor.Handle(ctx, job, workerID))
	return job
}

func installAuditFailureTrigger(t *testing.T, db *bun.DB, action string) {
	t.Helper()
	remove := installAuditFailureTriggerManual(t, db, action)
	t.Cleanup(remove)
}

func installAuditFailureTriggerManual(t *testing.T, db *bun.DB, action string) func() {
	t.Helper()
	ctx := context.Background()
	suffix := stringsForTrigger(action)
	functionName := "reject_database_audit_" + suffix
	triggerName := "reject_database_audit_trigger_" + suffix
	_, err := db.ExecContext(ctx, fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION %s() RETURNS trigger AS $$
		BEGIN
			IF NEW.action = %s THEN
				RAISE EXCEPTION 'forced managed database audit failure';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER %s
			BEFORE INSERT ON audit_logs
			FOR EACH ROW EXECUTE FUNCTION %s();
	`, functionName, postgresTestLiteral(action), triggerName, functionName))
	require.NoError(t, err)
	var once sync.Once
	remove := func() {
		once.Do(func() {
			_, dropErr := db.ExecContext(ctx, fmt.Sprintf(`
				DROP TRIGGER IF EXISTS %s ON audit_logs;
				DROP FUNCTION IF EXISTS %s();
			`, triggerName, functionName))
			require.NoError(t, dropErr)
		})
	}
	t.Cleanup(remove)
	return remove
}

func stringsForTrigger(value string) string {
	out := make([]byte, 0, len(value))
	for i := range len(value) {
		ch := value[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			out = append(out, ch)
		} else {
			out = append(out, '_')
		}
	}
	return string(out)
}

func postgresTestLiteral(value string) string {
	raw, _ := json.Marshal(value)
	return "'" + string(raw[1:len(raw)-1]) + "'"
}
