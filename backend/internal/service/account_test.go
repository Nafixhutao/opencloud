package service_test

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/database"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/repository"
	"github.com/nazxf/opencloud/backend/internal/service"
	"github.com/uptrace/bun"
)

type selectCounter struct {
	count atomic.Int64
}

func (h *selectCounter) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (h *selectCounter) AfterQuery(_ context.Context, event *bun.QueryEvent) {
	if event.Operation() == "SELECT" {
		h.count.Add(1)
	}
}

func openTestDB(t *testing.T) *bun.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration tests")
	}
	db, err := database.Connect(context.Background(), dsn)
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var n int
	err = db.NewRaw(`SELECT count(*) FROM information_schema.tables WHERE table_name = 'account_memberships'`).Scan(context.Background(), &n)
	if err != nil || n == 0 {
		t.Skip("account_memberships missing; run migrations first")
	}
	_, err = db.ExecContext(context.Background(), `
		CREATE SCHEMA IF NOT EXISTS auth;
		CREATE TABLE IF NOT EXISTS auth."user" (
			id text PRIMARY KEY,
			name text NOT NULL DEFAULT '',
			email text NOT NULL DEFAULT ''
		)`)
	require.NoError(t, err)
	return db
}

func newAccountService(t *testing.T, db *bun.DB) *service.AccountService {
	t.Helper()
	return service.NewAccountService(db, repository.NewAccountRepo(db), repository.NewAuditRepo(db))
}

func TestEnsureMembershipAndGetMe(t *testing.T) {
	db := openTestDB(t)
	svc := newAccountService(t, db)
	ctx := context.Background()

	userID := "user_" + uuid.NewString()
	me, err := svc.GetMe(ctx, userID, "Tenant A")
	require.NoError(t, err)
	require.Equal(t, userID, me.UserID)
	require.Equal(t, model.RoleCustomer, me.Role)
	require.Equal(t, model.MembershipActive, me.Status)
	require.NotEqual(t, uuid.Nil, me.AccountID)
	require.Equal(t, "Tenant A", me.Account.Name)

	me2, err := svc.GetMe(ctx, userID, "Ignored Name")
	require.NoError(t, err)
	require.Equal(t, me.AccountID, me2.AccountID)
}

func TestCrossTenantProfileUpdateRejected(t *testing.T) {
	db := openTestDB(t)
	svc := newAccountService(t, db)
	ctx := context.Background()

	a, err := svc.GetMe(ctx, "user_a_"+uuid.NewString(), "Account A")
	require.NoError(t, err)
	b, err := svc.GetMe(ctx, "user_b_"+uuid.NewString(), "Account B")
	require.NoError(t, err)
	require.NotEqual(t, a.AccountID, b.AccountID)

	_, err = svc.UpdateProfile(ctx, b.UserID, a.AccountID, service.UpdateProfileRequest{Name: "Hijacked"})
	require.Error(t, err)
	ae := apperr.As(err)
	require.NotNil(t, ae)
	require.Equal(t, "NOT_FOUND", ae.Code)

	updated, err := svc.UpdateProfile(ctx, a.UserID, a.AccountID, service.UpdateProfileRequest{Name: "Account A Renamed"})
	require.NoError(t, err)
	require.Equal(t, "Account A Renamed", updated.Account.Name)
}

func TestCustomerCannotBecomeAdminViaUpdate(t *testing.T) {
	db := openTestDB(t)
	svc := newAccountService(t, db)
	ctx := context.Background()

	me, err := svc.GetMe(ctx, "user_c_"+uuid.NewString(), "Customer")
	require.NoError(t, err)
	require.Equal(t, model.RoleCustomer, me.Role)

	admin, err := svc.BootstrapAdmin(ctx, me.UserID)
	require.NoError(t, err)
	require.Equal(t, model.RoleAdmin, admin.Role)

	admin2, err := svc.BootstrapAdmin(ctx, me.UserID)
	require.NoError(t, err)
	require.Equal(t, admin.MembershipID, admin2.MembershipID)
}

func TestAdminCannotDisableSelfOrLastAdmin(t *testing.T) {
	db := openTestDB(t)
	svc := newAccountService(t, db)
	ctx := context.Background()

	u1 := "admin1_" + uuid.NewString()
	u2 := "admin2_" + uuid.NewString()
	_, err := svc.GetMe(ctx, u1, "A1")
	require.NoError(t, err)
	_, err = svc.GetMe(ctx, u2, "A2")
	require.NoError(t, err)
	a1, err := svc.BootstrapAdmin(ctx, u1)
	require.NoError(t, err)
	a2, err := svc.BootstrapAdmin(ctx, u2)
	require.NoError(t, err)

	_, err = svc.UpdateUser(ctx, u1, a1.MembershipID, service.UpdateUserRequest{Status: model.MembershipDisabled})
	require.Error(t, err)
	require.Equal(t, "CONFLICT", apperr.As(err).Code)

	_, err = svc.UpdateUser(ctx, u1, a1.MembershipID, service.UpdateUserRequest{Role: model.RoleCustomer})
	require.Error(t, err)
	require.Equal(t, "CONFLICT", apperr.As(err).Code)

	_, err = svc.UpdateUser(ctx, u1, a2.MembershipID, service.UpdateUserRequest{Role: model.RoleCustomer})
	require.NoError(t, err)

	u3 := "admin3_" + uuid.NewString()
	_, err = svc.GetMe(ctx, u3, "A3")
	require.NoError(t, err)
	a3, err := svc.BootstrapAdmin(ctx, u3)
	require.NoError(t, err)

	_, err = svc.UpdateUser(ctx, u1, a3.MembershipID, service.UpdateUserRequest{Role: model.RoleCustomer})
	require.NoError(t, err)
}

func TestListUsersPagination(t *testing.T) {
	db := openTestDB(t)
	svc := newAccountService(t, db)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := svc.GetMe(ctx, "list_"+uuid.NewString(), "L")
		require.NoError(t, err)
	}
	counter := new(selectCounter)
	db.AddQueryHook(counter)
	users, total, err := svc.ListUsers(ctx, "admin_"+uuid.NewString(), 1, 2)
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, 3)
	require.LessOrEqual(t, len(users), 2)
	require.Equal(t, int64(2), counter.count.Load(), "admin list uses count + one joined page query")
}

func TestConcurrentAdminMutationsLeaveOneActiveAdmin(t *testing.T) {
	db := openTestDB(t)
	svc := newAccountService(t, db)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `UPDATE public.account_memberships SET role = 'customer'`)
	require.NoError(t, err)

	firstID := "race_admin_1_" + uuid.NewString()
	secondID := "race_admin_2_" + uuid.NewString()
	first, err := svc.BootstrapAdmin(ctx, firstID)
	require.NoError(t, err)
	second, err := svc.BootstrapAdmin(ctx, secondID)
	require.NoError(t, err)

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, membershipID := range []uuid.UUID{first.MembershipID, second.MembershipID} {
		wg.Add(1)
		go func(id uuid.UUID) {
			defer wg.Done()
			<-start
			_, updateErr := svc.UpdateUser(
				ctx,
				"independent_operator",
				id,
				service.UpdateUserRequest{Role: model.RoleCustomer},
			)
			errs <- updateErr
		}(membershipID)
	}
	close(start)
	wg.Wait()
	close(errs)

	successes := 0
	conflicts := 0
	for updateErr := range errs {
		if updateErr == nil {
			successes++
			continue
		}
		if ae := apperr.As(updateErr); ae != nil && ae.Code == "CONFLICT" {
			conflicts++
			continue
		}
		require.NoError(t, updateErr)
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)

	count, err := repository.NewAccountRepo(db).CountAdmins(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestEnsureMembershipConcurrentCallersConvergeWithoutOrphanAccount(t *testing.T) {
	db := openTestDB(t)
	repo := repository.NewAccountRepo(db)
	ctx := context.Background()
	userID := "membership_race_" + uuid.NewString()
	accountName := "Race Workspace " + uuid.NewString()

	const callers = 12
	results := make(chan *model.AccountMembership, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			membership, _, ensureErr := repo.EnsureMembership(ctx, userID, accountName)
			if ensureErr != nil {
				errs <- ensureErr
				return
			}
			results <- membership
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for ensureErr := range errs {
		require.NoError(t, ensureErr)
	}

	var winnerMembershipID, winnerAccountID uuid.UUID
	count := 0
	for membership := range results {
		if count == 0 {
			winnerMembershipID = membership.ID
			winnerAccountID = membership.AccountID
		}
		require.Equal(t, winnerMembershipID, membership.ID)
		require.Equal(t, winnerAccountID, membership.AccountID)
		count++
	}
	require.Equal(t, callers, count)

	var matchingAccounts int
	require.NoError(t, db.NewRaw(
		`SELECT count(*) FROM public.accounts WHERE name = ?`,
		accountName,
	).Scan(ctx, &matchingAccounts))
	require.Equal(t, 1, matchingAccounts)

	var winnerAccountRows int
	require.NoError(t, db.NewRaw(
		`SELECT count(*) FROM public.accounts WHERE id = ? AND name = ?`,
		winnerAccountID,
		accountName,
	).Scan(ctx, &winnerAccountRows))
	require.Equal(t, 1, winnerAccountRows)

	var orphanAccounts int
	require.NoError(t, db.NewRaw(`
		SELECT count(*)
		FROM public.accounts AS a
		LEFT JOIN public.account_memberships AS m ON m.account_id = a.id
		WHERE a.name = ? AND m.id IS NULL
	`, accountName).Scan(ctx, &orphanAccounts))
	require.Zero(t, orphanAccounts)
}

func TestProfileMutationRollsBackWhenAuditInsertFails(t *testing.T) {
	db := openTestDB(t)
	svc := newAccountService(t, db)
	ctx := context.Background()
	me, err := svc.GetMe(ctx, "audit_profile_"+uuid.NewString(), "Original")
	require.NoError(t, err)

	installFailingAuditTrigger(t, db)
	_, err = svc.UpdateProfile(
		ctx,
		me.UserID,
		me.AccountID,
		service.UpdateProfileRequest{Name: "Must Roll Back"},
	)
	require.Error(t, err)

	account, err := repository.NewAccountRepo(db).GetAccountByID(ctx, me.AccountID)
	require.NoError(t, err)
	require.Equal(t, "Original", account.Name)
}

func TestPrivilegedMutationRollsBackWhenAuditInsertFails(t *testing.T) {
	db := openTestDB(t)
	svc := newAccountService(t, db)
	ctx := context.Background()
	target, err := svc.GetMe(ctx, "audit_admin_"+uuid.NewString(), "Target")
	require.NoError(t, err)
	membership, err := repository.NewAccountRepo(db).GetMembershipByUserID(ctx, target.UserID)
	require.NoError(t, err)

	installFailingAuditTrigger(t, db)
	_, err = svc.UpdateUser(
		ctx,
		"platform_operator",
		membership.ID,
		service.UpdateUserRequest{Role: model.RoleAdmin},
	)
	require.Error(t, err)

	reloaded, err := repository.NewAccountRepo(db).GetMembershipByID(ctx, membership.ID)
	require.NoError(t, err)
	require.Equal(t, model.RoleCustomer, reloaded.Role)
}

func installFailingAuditTrigger(t *testing.T, db *bun.DB) {
	t.Helper()
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION public.test_fail_audit_insert()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			RAISE EXCEPTION 'forced audit failure';
		END;
		$$;
		DROP TRIGGER IF EXISTS test_fail_audit_insert ON public.audit_logs;
		CREATE TRIGGER test_fail_audit_insert
		BEFORE INSERT ON public.audit_logs
		FOR EACH ROW EXECUTE FUNCTION public.test_fail_audit_insert()`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, cleanupErr := db.ExecContext(context.Background(), `
			DROP TRIGGER IF EXISTS test_fail_audit_insert ON public.audit_logs;
			DROP FUNCTION IF EXISTS public.test_fail_audit_insert()`)
		require.NoError(t, cleanupErr)
	})
}
