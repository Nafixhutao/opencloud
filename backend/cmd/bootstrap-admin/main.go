// Command bootstrap-admin promotes a user to role=admin by user id.
// Idempotent: re-running on an existing admin is a no-op success.
//
//	DATABASE_URL=… go run ./cmd/bootstrap-admin --user-id <better-auth-user-id>
//
// Never expose this over the public HTTP API. Operators run it on the
// control-plane host with DB access (docs/SECURITY.md, docs/DEPLOYMENT.md).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/app"
	"github.com/nazxf/opencloud/backend/internal/repository"
	"github.com/nazxf/opencloud/backend/internal/service"
)

func main() {
	userID := flag.String("user-id", "", "better-auth user id to promote to admin")
	flag.Parse()
	if *userID == "" {
		fmt.Fprintln(os.Stderr, "usage: bootstrap-admin --user-id <id>")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	deps, err := app.Bootstrap(ctx, true)
	if err != nil {
		panic(err)
	}
	defer deps.Close()

	svc := service.NewAccountService(
		deps.DB,
		repository.NewAccountRepo(deps.DB),
		repository.NewAuditRepo(deps.DB),
	)
	user, err := svc.BootstrapAdmin(ctx, *userID)
	if err != nil {
		deps.Log.Fatal("bootstrap admin failed", zap.Error(err))
	}
	deps.Log.Info("admin ready",
		zap.String("user_id", user.UserID),
		zap.String("membership_id", user.MembershipID.String()),
		zap.String("role", user.Role),
		zap.String("status", user.Status),
	)
	fmt.Printf("ok user_id=%s role=%s status=%s\n", user.UserID, user.Role, user.Status)
}
