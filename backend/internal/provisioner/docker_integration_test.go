//go:build integration

package provisioner

import (
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestDockerCaddyLifecycleAgainstDisposableBackend(t *testing.T) {
	if os.Getenv("DOCKER_INTEGRATION") != "1" {
		t.Skip("DOCKER_INTEGRATION is not enabled")
	}
	image := os.Getenv("DOCKER_SITE_IMAGE")
	caddyURL := os.Getenv("CADDY_INTEGRATION_URL")
	publicURL := os.Getenv("CADDY_INTEGRATION_PUBLIC_URL")
	if image == "" || caddyURL == "" || publicURL == "" {
		t.Fatal("Docker integration environment is incomplete")
	}

	adapter, err := NewDocker("/var/run/docker.sock", caddyURL, "srv0", image)
	require.NoError(t, err)
	spec := SiteSpec{
		SiteID:       uuid.MustParse("00000000-0000-4000-8000-00000000d257"),
		AccountID:    uuid.MustParse("00000000-0000-4000-8000-00000000a257"),
		NodeID:       uuid.MustParse("00000000-0000-4000-8000-00000000b257"),
		Domain:       "phase2-validation.example.test",
		Image:        image,
		InternalPort: 8080,
		MemoryBytes:  128 * 1024 * 1024,
		NanoCPUs:     250_000_000,
	}
	ref := SiteRef{SiteID: spec.SiteID, AccountID: spec.AccountID, NodeID: spec.NodeID}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cleanupCancel()
		_ = adapter.DeleteSite(cleanupCtx, ref)
	})

	require.NoError(t, adapter.CreateSite(ctx, spec))
	require.NoError(t, adapter.CreateSite(ctx, spec), "second create must converge")
	require.Equal(t, SiteStateRunning, mustSiteState(t, ctx, adapter, ref))
	require.Equal(t, http.StatusOK, requestSite(t, ctx, publicURL, spec.Domain))

	require.NoError(t, adapter.SuspendSite(ctx, ref))
	require.NoError(t, adapter.SuspendSite(ctx, ref), "second suspend must converge")
	require.Equal(t, SiteStateSuspended, mustSiteState(t, ctx, adapter, ref))

	require.NoError(t, adapter.ResumeSite(ctx, ref))
	require.NoError(t, adapter.ResumeSite(ctx, ref), "second resume must converge")
	require.Equal(t, SiteStateRunning, mustSiteState(t, ctx, adapter, ref))
	require.Equal(t, http.StatusOK, requestSite(t, ctx, publicURL, spec.Domain))

	require.NoError(t, adapter.DeleteSite(ctx, ref))
	require.NoError(t, adapter.DeleteSite(ctx, ref), "second delete must be a no-op")
	require.Equal(t, SiteStateMissing, mustSiteState(t, ctx, adapter, ref))
}

func mustSiteState(
	t *testing.T,
	ctx context.Context,
	adapter *Docker,
	ref SiteRef,
) SiteState {
	t.Helper()
	state, err := adapter.SiteStatus(ctx, ref)
	require.NoError(t, err)
	return state
}

func requestSite(t *testing.T, ctx context.Context, endpoint, domain string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	require.NoError(t, err)
	req.Host = domain
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return response.StatusCode
}
