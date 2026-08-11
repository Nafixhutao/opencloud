package provisioner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CaddyRouter is the deep module for Caddy route management. It owns the ETag
// concurrency guard, route shape, hostname normalization, route ownership
// validation, and deterministic route IDs — separated from the Docker runtime
// adapter per ADR 0008 (Caddy is the routing backend, Docker is the runtime).
//
// The Docker provisioner composes this module: it inspects the container for
// the published port, then calls UpsertRoute/DeleteRoute. Caddy ETag retry
// logic no longer lives between container inspection calls.
type CaddyRouter struct {
	client   *http.Client
	url      *url.URL
	serverID string
}

// NewCaddyRouter validates the Caddy admin endpoint and constructs the adapter.
func NewCaddyRouter(caddyAPIURL, caddyServerID string) (*CaddyRouter, error) {
	caddyURL, err := url.Parse(strings.TrimSpace(caddyAPIURL))
	if err != nil || (caddyURL.Scheme != "http" && caddyURL.Scheme != "https") || caddyURL.Host == "" {
		return nil, errors.New("invalid Caddy admin URL")
	}
	if strings.TrimSpace(caddyServerID) == "" {
		caddyServerID = "srv0"
	}
	return &CaddyRouter{
		client:   &http.Client{Timeout: 15 * time.Second},
		url:      caddyURL,
		serverID: caddyServerID,
	}, nil
}

// UpsertRoute creates or updates the owned Caddy route for a site. When
// preserveExisting is true, existing hostnames on a matching route are merged
// with the new set — used by SetSiteDomains. The ETag guard retries on
// concurrent modifications (StatusPreconditionFailed).
func (c *CaddyRouter) UpsertRoute(ctx context.Context, siteID uuid.UUID, hostnames []string, hostPort string, preserveExisting bool) error {
	hostnames, err := normalizeHostnames(hostnames)
	if err != nil {
		return err
	}
	route := caddyRoute{
		ID:    caddyRouteID(siteID),
		Match: []map[string][]string{{"host": hostnames}},
		Handle: []caddyHandler{{
			Handler:   "reverse_proxy",
			Upstreams: []caddyUpstream{{Dial: "127.0.0.1:" + hostPort}},
		}},
		Terminal: true,
	}
	for attempt := 0; attempt < caddyRetries; attempt++ {
		var existing caddyRoute
		status, etag, err := c.caddyRequest(ctx, http.MethodGet, "/id/"+url.PathEscape(route.ID), nil, "", &existing)
		if err != nil {
			return err
		}
		if status == http.StatusOK {
			if !routeOwned(existing, route.ID, "") {
				return errors.New("refusing to replace mismatched Caddy route")
			}
			if preserveExisting {
				route.Match[0]["host"], err = normalizeHostnames(append(
					append([]string(nil), route.Match[0]["host"]...),
					existing.Match[0]["host"]...,
				))
				if err != nil {
					return err
				}
			}
			status, _, err = c.caddyRequest(ctx, http.MethodPatch, "/id/"+url.PathEscape(route.ID), route, etag, nil)
			if err != nil {
				return err
			}
			if status >= 200 && status < 300 {
				return nil
			}
			if status == http.StatusPreconditionFailed {
				continue
			}
			return unexpectedStatus(status)
		}
		if status != http.StatusNotFound {
			return unexpectedStatus(status)
		}

		routesPath := "/config/apps/http/servers/" + url.PathEscape(c.serverID) + "/routes"
		status, etag, err = c.caddyRequest(ctx, http.MethodGet, routesPath, nil, "", nil)
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return fmt.Errorf("caddy server routes unavailable: %w", unexpectedStatus(status))
		}
		status, _, err = c.caddyRequest(ctx, http.MethodPost, routesPath, route, etag, nil)
		if err != nil {
			return err
		}
		if status >= 200 && status < 300 {
			return nil
		}
		if status != http.StatusPreconditionFailed {
			return unexpectedStatus(status)
		}
	}
	return errors.New("caddy config changed concurrently too many times")
}

// DeleteRoute removes the owned Caddy route for a site. When expectedDomain is
// non-empty, the route is only deleted if it matches that domain — a guard
// against deleting a route that has been reassigned.
func (c *CaddyRouter) DeleteRoute(ctx context.Context, siteID uuid.UUID, expectedDomain string) error {
	id := caddyRouteID(siteID)
	for attempt := 0; attempt < caddyRetries; attempt++ {
		var route caddyRoute
		status, etag, err := c.caddyRequest(ctx, http.MethodGet, "/id/"+url.PathEscape(id), nil, "", &route)
		if err != nil {
			return err
		}
		if status == http.StatusNotFound {
			return nil
		}
		if status != http.StatusOK {
			return unexpectedStatus(status)
		}
		if !routeOwned(route, id, expectedDomain) {
			return errors.New("refusing to delete mismatched Caddy route")
		}
		status, _, err = c.caddyRequest(ctx, http.MethodDelete, "/id/"+url.PathEscape(id), nil, etag, nil)
		if err != nil {
			return err
		}
		if status >= 200 && status < 300 || status == http.StatusNotFound {
			return nil
		}
		if status != http.StatusPreconditionFailed {
			return unexpectedStatus(status)
		}
	}
	return errors.New("caddy config changed concurrently too many times")
}

func (c *CaddyRouter) caddyRequest(
	ctx context.Context,
	method, requestPath string,
	body any,
	ifMatch string,
	out any,
) (int, string, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, "", err
		}
		reader = bytes.NewReader(raw)
	}
	endpoint := *c.url
	endpoint.Path = strings.TrimRight(c.url.Path, "/") + requestPath
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
	if err != nil {
		return 0, "", err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && out != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out); err != nil {
			return resp.StatusCode, resp.Header.Get("Etag"), err
		}
	}
	return resp.StatusCode, resp.Header.Get("Etag"), nil
}

type caddyRoute struct {
	ID       string                `json:"@id"`
	Match    []map[string][]string `json:"match"`
	Handle   []caddyHandler        `json:"handle"`
	Terminal bool                  `json:"terminal"`
}

type caddyHandler struct {
	Handler   string          `json:"handler"`
	Upstreams []caddyUpstream `json:"upstreams"`
}

type caddyUpstream struct {
	Dial string `json:"dial"`
}

func routeOwned(route caddyRoute, id, domain string) bool {
	domainMatches := domain == ""
	if !domainMatches && len(route.Match) == 1 {
		for _, hostname := range route.Match[0]["host"] {
			if hostname == domain {
				domainMatches = true
				break
			}
		}
	}
	return route.ID == id &&
		domainMatches &&
		len(route.Match) == 1 &&
		len(route.Match[0]["host"]) > 0 &&
		len(route.Handle) == 1 &&
		route.Handle[0].Handler == "reverse_proxy" &&
		len(route.Handle[0].Upstreams) == 1 &&
		strings.HasPrefix(route.Handle[0].Upstreams[0].Dial, "127.0.0.1:")
}

func normalizeHostnames(values []string) ([]string, error) {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		hostname := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
		if len(hostname) < 3 || len(hostname) > 253 || !strings.Contains(hostname, ".") {
			return nil, errors.New("invalid route hostname")
		}
		for _, label := range strings.Split(hostname, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return nil, errors.New("invalid route hostname")
			}
			for _, character := range label {
				if (character < 'a' || character > 'z') &&
					(character < '0' || character > '9') && character != '-' {
					return nil, errors.New("invalid route hostname")
				}
			}
		}
		unique[hostname] = struct{}{}
	}
	if len(unique) == 0 {
		return nil, errors.New("at least one route hostname is required")
	}
	hostnames := make([]string, 0, len(unique))
	for hostname := range unique {
		hostnames = append(hostnames, hostname)
	}
	sort.Strings(hostnames)
	return hostnames, nil
}

func caddyRouteID(siteID uuid.UUID) string {
	return ResourceName(siteID) + "-route"
}

func unexpectedStatus(status int) error {
	return fmt.Errorf("unexpected provider status %d", status)
}
