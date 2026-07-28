// Command api runs the OpenCloud HTTP control-plane server.
package main

import (
	"context"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/app"
	"github.com/nazxf/opencloud/backend/internal/credential"
	"github.com/nazxf/opencloud/backend/internal/metrics"
	"github.com/nazxf/opencloud/backend/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	deps, err := app.Bootstrap(ctx, true)
	if err != nil {
		// Logger may not exist yet; fail loudly on stderr.
		panic(err)
	}
	if err := deps.Cfg.ValidateAPI(); err != nil {
		deps.Close()
		panic(err)
	}
	defer deps.Close()

	var databaseCipher *credential.Cipher
	if deps.Cfg.CustomerDatabases.Enabled {
		databaseCipher, err = credential.New(deps.Cfg.CustomerDatabases.CredentialKey)
		if err != nil {
			deps.Log.Fatal("initialize customer database credential cipher", zap.Error(err))
		}
	}

	m := metrics.New()
	srv := server.New(deps.Cfg, deps.Log, deps.DB, deps.RDB, m, databaseCipher)

	if err := srv.Run(ctx); err != nil {
		deps.Log.Fatal("api exited", zap.Error(err))
	}
}
