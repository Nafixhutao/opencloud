package provisioner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testResponse(status int, body string, headers map[string]string) *http.Response {
	header := make(http.Header)
	for key, value := range headers {
		header.Set(key, value)
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestDockerCreateSiteIsHardenedIdempotentAndCaddySafe(t *testing.T) {
	spec := SiteSpec{
		SiteID:       uuid.New(),
		AccountID:    uuid.New(),
		NodeID:       uuid.New(),
		Domain:       "site.example.test",
		Image:        "opencloud/site-static:phase2",
		InternalPort: 8080,
		MemoryBytes:  256 * 1024 * 1024,
		NanoCPUs:     500_000_000,
	}
	labels := ownershipLabels(spec.AccountID, spec.SiteID, spec.NodeID)
	containerLabels := cloneLabels(labels)
	containerLabels["opencloud.domain"] = spec.Domain

	var mu sync.Mutex
	networkCreated := false
	volumeCreated := false
	containerCreated := false
	containerCreates := 0
	var createBody map[string]any
	engine := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		requestPath := req.URL.Path
		switch {
		case strings.Contains(requestPath, "/networks/") && req.Method == http.MethodGet:
			if !networkCreated {
				return testResponse(http.StatusNotFound, "", nil), nil
			}
			raw, _ := json.Marshal(map[string]any{"Labels": labels})
			return testResponse(http.StatusOK, string(raw), nil), nil
		case strings.HasSuffix(requestPath, "/networks/create"):
			networkCreated = true
			return testResponse(http.StatusCreated, `{}`, nil), nil
		case strings.Contains(requestPath, "/volumes/") && req.Method == http.MethodGet:
			if !volumeCreated {
				return testResponse(http.StatusNotFound, "", nil), nil
			}
			raw, _ := json.Marshal(map[string]any{"Labels": labels})
			return testResponse(http.StatusOK, string(raw), nil), nil
		case strings.HasSuffix(requestPath, "/volumes/create"):
			volumeCreated = true
			return testResponse(http.StatusCreated, `{}`, nil), nil
		case strings.Contains(requestPath, "/containers/") && strings.HasSuffix(requestPath, "/json"):
			if !containerCreated {
				return testResponse(http.StatusNotFound, "", nil), nil
			}
			raw, _ := json.Marshal(map[string]any{
				"Config": map[string]any{"Labels": containerLabels},
				"State":  map[string]any{"Running": true},
				"NetworkSettings": map[string]any{
					"Ports": map[string]any{
						"8080/tcp": []map[string]string{{"HostIp": "127.0.0.1", "HostPort": "32768"}},
					},
				},
			})
			return testResponse(http.StatusOK, string(raw), nil), nil
		case strings.HasSuffix(requestPath, "/containers/create"):
			containerCreates++
			require.NoError(t, json.NewDecoder(req.Body).Decode(&createBody))
			containerCreated = true
			return testResponse(http.StatusCreated, `{}`, nil), nil
		case strings.HasSuffix(requestPath, "/start"):
			return testResponse(http.StatusNoContent, "", nil), nil
		default:
			t.Fatalf("unexpected Engine request %s %s", req.Method, requestPath)
			return nil, nil
		}
	})}

	var route *caddyRoute
	caddyPosts := 0
	caddyPatches := 0
	caddy := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.HasPrefix(req.URL.Path, "/id/") && req.Method == http.MethodGet:
			if route == nil {
				return testResponse(http.StatusNotFound, "", nil), nil
			}
			raw, _ := json.Marshal(route)
			return testResponse(http.StatusOK, string(raw), map[string]string{"Etag": `"route-v1"`}), nil
		case strings.HasPrefix(req.URL.Path, "/id/") && req.Method == http.MethodPatch:
			require.Equal(t, `"route-v1"`, req.Header.Get("If-Match"))
			var updated caddyRoute
			require.NoError(t, json.NewDecoder(req.Body).Decode(&updated))
			route = &updated
			caddyPatches++
			return testResponse(http.StatusOK, `{}`, nil), nil
		case strings.HasSuffix(req.URL.Path, "/routes") && req.Method == http.MethodGet:
			return testResponse(http.StatusOK, `[]`, map[string]string{"Etag": `"routes-v1"`}), nil
		case strings.HasSuffix(req.URL.Path, "/routes") && req.Method == http.MethodPost:
			require.Equal(t, `"routes-v1"`, req.Header.Get("If-Match"))
			var created caddyRoute
			require.NoError(t, json.NewDecoder(req.Body).Decode(&created))
			route = &created
			caddyPosts++
			return testResponse(http.StatusOK, `{}`, nil), nil
		default:
			t.Fatalf("unexpected Caddy request %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	})}

	caddyURL, err := url.Parse("http://caddy.test")
	if err != nil {
		t.Fatal(err)
	}
	adapter := &Docker{
		engine:       engine,
		caddy:        caddy,
		caddyURL:     caddyURL,
		serverID:     "srv0",
		allowedImage: spec.Image,
	}

	require.NoError(t, adapter.CreateSite(context.Background(), spec))
	require.NoError(t, adapter.CreateSite(context.Background(), spec))
	require.Equal(t, 1, containerCreates)
	require.Equal(t, 1, caddyPosts)
	require.Equal(t, 1, caddyPatches)

	hostConfig, ok := createBody["HostConfig"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, hostConfig["ReadonlyRootfs"])
	require.Equal(t, []any{"ALL"}, hostConfig["CapDrop"])
	require.Equal(t, []any{"no-new-privileges:true"}, hostConfig["SecurityOpt"])
	require.EqualValues(t, spec.MemoryBytes, hostConfig["Memory"])
	require.EqualValues(t, spec.NanoCPUs, hostConfig["NanoCpus"])
	require.EqualValues(t, 128, hostConfig["PidsLimit"])
	require.NotContains(t, hostConfig, "Privileged")
	require.NotContains(t, hostConfig, "Binds")
	require.Equal(t, spec.Domain, route.Match[0]["host"][0])
	require.Equal(t, "127.0.0.1:32768", route.Handle[0].Upstreams[0].Dial)
}

func TestDockerDeleteRefusesMismatchedOwnership(t *testing.T) {
	ref := SiteRef{SiteID: uuid.New(), AccountID: uuid.New(), NodeID: uuid.New()}
	deleteCalls := 0
	engine := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodDelete {
			deleteCalls++
		}
		raw, _ := json.Marshal(map[string]any{
			"Config": map[string]any{"Labels": map[string]string{
				managedLabelKey: "true",
				accountLabelKey: ref.AccountID.String(),
				siteLabelKey:    uuid.NewString(),
				nodeLabelKey:    ref.NodeID.String(),
			}},
		})
		return testResponse(http.StatusOK, string(raw), nil), nil
	})}
	caddyURL, _ := url.Parse("http://caddy.test")
	adapter := &Docker{
		engine: engine,
		caddy: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("Caddy must not be touched after ownership mismatch")
			return nil, nil
		})},
		caddyURL:     caddyURL,
		serverID:     "srv0",
		allowedImage: "opencloud/site-static:phase2",
	}

	err := adapter.DeleteSite(context.Background(), ref)
	require.Error(t, err)
	require.Zero(t, deleteCalls)
}

func TestDockerSetSiteDomainsPreservesExactHostsAndOwnership(t *testing.T) {
	ref := SiteRef{SiteID: uuid.New(), AccountID: uuid.New(), NodeID: uuid.New()}
	labels := ownershipLabels(ref.AccountID, ref.SiteID, ref.NodeID)
	container := func(labels map[string]string) string {
		raw, err := json.Marshal(map[string]any{
			"Config": map[string]any{"Labels": labels},
			"NetworkSettings": map[string]any{
				"Ports": map[string]any{
					"8080/tcp": []map[string]string{{"HostIp": "127.0.0.1", "HostPort": "32768"}},
				},
			},
		})
		require.NoError(t, err)
		return string(raw)
	}

	t.Run("writes the complete deduplicated matcher", func(t *testing.T) {
		engine := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return testResponse(http.StatusOK, container(labels), nil), nil
		})}
		existing := caddyRoute{
			ID:    caddyRouteID(ref.SiteID),
			Match: []map[string][]string{{"host": {"primary.example.test"}}},
			Handle: []caddyHandler{{
				Handler:   "reverse_proxy",
				Upstreams: []caddyUpstream{{Dial: "127.0.0.1:32768"}},
			}},
			Terminal: true,
		}
		var updated caddyRoute
		caddy := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.Method {
			case http.MethodGet:
				raw, err := json.Marshal(existing)
				require.NoError(t, err)
				return testResponse(http.StatusOK, string(raw), map[string]string{"Etag": `"route-v1"`}), nil
			case http.MethodPatch:
				require.Equal(t, `"route-v1"`, req.Header.Get("If-Match"))
				require.NoError(t, json.NewDecoder(req.Body).Decode(&updated))
				return testResponse(http.StatusOK, `{}`, nil), nil
			default:
				t.Fatalf("unexpected Caddy request %s", req.Method)
				return nil, nil
			}
		})}
		caddyURL, err := url.Parse("http://caddy.test")
		require.NoError(t, err)
		adapter := &Docker{engine: engine, caddy: caddy, caddyURL: caddyURL, serverID: "srv0"}

		err = adapter.SetSiteDomains(context.Background(), ref, []string{
			"www.example.test", "primary.example.test", "api.example.test", "www.example.test",
		})
		require.NoError(t, err)
		require.Equal(t, []string{"api.example.test", "primary.example.test", "www.example.test"}, updated.Match[0]["host"])
		require.Equal(t, caddyRouteID(ref.SiteID), updated.ID)
		require.Equal(t, "127.0.0.1:32768", updated.Handle[0].Upstreams[0].Dial)
	})

	t.Run("refuses a container owned by another site", func(t *testing.T) {
		foreign := cloneLabels(labels)
		foreign[siteLabelKey] = uuid.NewString()
		engine := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return testResponse(http.StatusOK, container(foreign), nil), nil
		})}
		caddyURL, err := url.Parse("http://caddy.test")
		require.NoError(t, err)
		adapter := &Docker{
			engine: engine,
			caddy: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("Caddy must not be touched after ownership mismatch")
				return nil, nil
			})},
			caddyURL: caddyURL,
			serverID: "srv0",
		}

		err = adapter.SetSiteDomains(context.Background(), ref, []string{"primary.example.test"})
		require.ErrorContains(t, err, "ownership label")
	})

	t.Run("an empty matcher removes the owned route", func(t *testing.T) {
		engine := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return testResponse(http.StatusOK, container(labels), nil), nil
		})}
		deleted := false
		caddy := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.Method {
			case http.MethodGet:
				existing := caddyRoute{
					ID:    caddyRouteID(ref.SiteID),
					Match: []map[string][]string{{"host": {"primary.example.test"}}},
					Handle: []caddyHandler{{
						Handler:   "reverse_proxy",
						Upstreams: []caddyUpstream{{Dial: "127.0.0.1:32768"}},
					}},
				}
				raw, err := json.Marshal(existing)
				require.NoError(t, err)
				return testResponse(http.StatusOK, string(raw), map[string]string{"Etag": `"route-v1"`}), nil
			case http.MethodDelete:
				require.Equal(t, `"route-v1"`, req.Header.Get("If-Match"))
				deleted = true
				return testResponse(http.StatusOK, `{}`, nil), nil
			default:
				t.Fatalf("unexpected Caddy request %s", req.Method)
				return nil, nil
			}
		})}
		caddyURL, err := url.Parse("http://caddy.test")
		require.NoError(t, err)
		adapter := &Docker{engine: engine, caddy: caddy, caddyURL: caddyURL, serverID: "srv0"}

		require.NoError(t, adapter.SetSiteDomains(context.Background(), ref, nil))
		require.True(t, deleted)
	})
}

func TestNewDockerRejectsNonUnixSocketAndInvalidCaddyURL(t *testing.T) {
	_, err := NewDocker("tcp://127.0.0.1:2375", "http://127.0.0.1:2019", "srv0", "image")
	require.Error(t, err)
	_, err = NewDocker("/var/run/docker.sock", "ftp://caddy.invalid", "srv0", "image")
	require.Error(t, err)
}

func TestDockerCertificateStatusDialsConfiguredIngressWithHostname(t *testing.T) {
	wantExpiry := time.Now().UTC().Add(90 * 24 * time.Hour).Truncate(time.Second)
	var gotHostname, gotIngress string
	adapter := &Docker{certificateProbe: func(
		_ context.Context,
		hostname, ingressIPv4 string,
	) (CertificateObservation, error) {
		gotHostname = hostname
		gotIngress = ingressIPv4
		return CertificateObservation{ExpiresAt: wantExpiry}, nil
	}}

	observation, err := adapter.CertificateStatus(
		context.Background(), "www.example.com", "8.8.8.8",
	)
	require.NoError(t, err)
	require.Equal(t, "www.example.com", gotHostname)
	require.Equal(t, "8.8.8.8", gotIngress)
	require.Equal(t, wantExpiry, observation.ExpiresAt)

	_, err = adapter.CertificateStatus(context.Background(), "www.example.com", "10.0.0.1")
	require.Error(t, err)
}

func TestEngineRequestDoesNotDecodeProviderErrors(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return testResponse(http.StatusInternalServerError, `{"message":"do not persist this provider response"}`, nil), nil
	})}
	adapter := &Docker{engine: client}
	var out bytes.Buffer
	status, err := adapter.engineRequestStatus(context.Background(), http.MethodGet, "/info", nil, &out)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, status)
	require.Zero(t, out.Len())
}
