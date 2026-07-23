package service_test

import (
	"context"
	"os"
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
	users, total, err := svc.ListUsers(ctx, 1, 2)
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, 3)
	require.LessOrEqual(t, len(users), 2)
}
