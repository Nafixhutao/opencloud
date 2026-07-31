package provisioner

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	dockerAPIVersion = "v1.46"
	managedLabelKey  = "opencloud.managed"
	accountLabelKey  = "opencloud.account_id"
	siteLabelKey     = "opencloud.site_id"
	nodeLabelKey     = "opencloud.node_id"
	caddyRetries     = 5
)

// Docker provisions hardened site containers and isolated resources through a
// permissioned Unix socket, then maintains one owned Caddy route per site.
type Docker struct {
	engine           *http.Client
	caddy            *http.Client
	caddyURL         *url.URL
	serverID         string
	allowedImage     string
	certificateProbe func(context.Context, string, string) (CertificateObservation, error)
}

// NewDocker validates local worker configuration and constructs the adapter.
func NewDocker(socketPath, caddyAPIURL, caddyServerID, allowedImage string) (*Docker, error) {
	socketPath = strings.TrimPrefix(strings.TrimSpace(socketPath), "unix://")
	if socketPath == "" || !path.IsAbs(socketPath) {
		return nil, errors.New("Docker socket must be an absolute Unix socket path")
	}
	caddyURL, err := url.Parse(strings.TrimSpace(caddyAPIURL))
	if err != nil || (caddyURL.Scheme != "http" && caddyURL.Scheme != "https") || caddyURL.Host == "" {
		return nil, errors.New("invalid Caddy admin URL")
	}
	if strings.TrimSpace(caddyServerID) == "" {
		caddyServerID = "srv0"
	}
	if strings.TrimSpace(allowedImage) == "" {
		return nil, errors.New("allowed site image is required")
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
		DisableCompression: true,
	}
	return &Docker{
		engine: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		caddy:            &http.Client{Timeout: 15 * time.Second},
		caddyURL:         caddyURL,
		serverID:         caddyServerID,
		allowedImage:     allowedImage,
		certificateProbe: probeCertificate,
	}, nil
}

// CreateSite converges every owned Docker object, starts the container, and
// creates its Caddy route.
func (d *Docker) CreateSite(ctx context.Context, spec SiteSpec) error {
	if err := d.validateSpec(spec); err != nil {
		return err
	}
	labels := ownershipLabels(spec.AccountID, spec.SiteID, spec.NodeID)
	name := ResourceName(spec.SiteID)
	networkName := name + "-net"
	volumeName := name + "-data"

	if err := d.ensureNetwork(ctx, networkName, labels); err != nil {
		return fmt.Errorf("ensure network: %w", err)
	}
	if err := d.ensureVolume(ctx, volumeName, labels); err != nil {
		return fmt.Errorf("ensure volume: %w", err)
	}

	container, found, err := d.inspectContainer(ctx, name)
	if err != nil {
		return fmt.Errorf("inspect container: %w", err)
	}
	if found {
		if err := requireOwnership(container.Config.Labels, labels); err != nil {
			return err
		}
	} else {
		if err := d.createContainer(ctx, name, networkName, volumeName, labels, spec); err != nil {
			return fmt.Errorf("create container: %w", err)
		}
	}
	if err := d.startContainer(ctx, name); err != nil {
		return fmt.Errorf("start container: %w", err)
	}
	container, found, err = d.inspectContainer(ctx, name)
	if err != nil || !found {
		return fmt.Errorf("inspect started container: %w", err)
	}
	hostPort, err := publishedPort(container, spec.InternalPort)
	if err != nil {
		return err
	}
	if err := d.upsertCaddyRoute(ctx, spec, hostPort); err != nil {
		return fmt.Errorf("configure Caddy route: %w", err)
	}
	return nil
}

// DeleteSite removes only resources whose complete ownership labels match.
// Missing resources are success.
func (d *Docker) DeleteSite(ctx context.Context, ref SiteRef) error {
	name := ResourceName(ref.SiteID)
	labels := ownershipLabels(ref.AccountID, ref.SiteID, ref.NodeID)
	container, found, err := d.inspectContainer(ctx, name)
	if err != nil {
		return err
	}
	if found {
		if err := requireOwnership(container.Config.Labels, labels); err != nil {
			return err
		}
	}
	if err := d.deleteCaddyRoute(ctx, ref.SiteID, ""); err != nil {
		return err
	}
	if found {
		if err := d.engineRequest(ctx, http.MethodDelete, "/containers/"+url.PathEscape(name)+"?force=true&v=false", nil, nil, http.StatusNoContent, http.StatusNotFound); err != nil {
			return err
		}
	}
	if err := d.removeNetwork(ctx, name+"-net", labels); err != nil {
		return err
	}
	if err := d.removeVolume(ctx, name+"-data", labels); err != nil {
		return err
	}
	return nil
}

// SuspendSite removes public routing and stops the owned container.
func (d *Docker) SuspendSite(ctx context.Context, ref SiteRef) error {
	name := ResourceName(ref.SiteID)
	container, found, err := d.inspectContainer(ctx, name)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("site container is missing")
	}
	if err := requireOwnership(container.Config.Labels, ownershipLabels(ref.AccountID, ref.SiteID, ref.NodeID)); err != nil {
		return err
	}
	if err := d.deleteCaddyRoute(ctx, ref.SiteID, ""); err != nil {
		return err
	}
	return d.engineRequest(ctx, http.MethodPost, "/containers/"+url.PathEscape(name)+"/stop?t=20", nil, nil, http.StatusNoContent, http.StatusNotModified)
}

// ResumeSite starts the owned container and restores its Caddy route.
func (d *Docker) ResumeSite(ctx context.Context, ref SiteRef) error {
	name := ResourceName(ref.SiteID)
	container, found, err := d.inspectContainer(ctx, name)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("site container is missing")
	}
	if err := requireOwnership(container.Config.Labels, ownershipLabels(ref.AccountID, ref.SiteID, ref.NodeID)); err != nil {
		return err
	}
	if err := d.startContainer(ctx, name); err != nil {
		return err
	}
	container, _, err = d.inspectContainer(ctx, name)
	if err != nil {
		return err
	}
	port, err := firstPublishedPort(container)
	if err != nil {
		return err
	}
	domain := container.Config.Labels["opencloud.domain"]
	if domain == "" {
		return errors.New("owned container has no domain label")
	}
	return d.upsertCaddyRoute(ctx, SiteSpec{
		SiteID:    ref.SiteID,
		AccountID: ref.AccountID,
		NodeID:    ref.NodeID,
		Domain:    domain,
	}, port)
}

// SiteStatus reports missing, running, or suspended after ownership validation.
func (d *Docker) SiteStatus(ctx context.Context, ref SiteRef) (SiteState, error) {
	container, found, err := d.inspectContainer(ctx, ResourceName(ref.SiteID))
	if err != nil {
		return "", err
	}
	if !found {
		return SiteStateMissing, nil
	}
	if err := requireOwnership(container.Config.Labels, ownershipLabels(ref.AccountID, ref.SiteID, ref.NodeID)); err != nil {
		return "", err
	}
	if container.State.Running {
		return SiteStateRunning, nil
	}
	return SiteStateSuspended, nil
}

// SetSiteDomains replaces the hostname matcher on one owned site route. The
// complete set is written atomically with Caddy's ETag guard, so concurrent
// domain jobs retry instead of dropping another hostname.
func (d *Docker) SetSiteDomains(ctx context.Context, ref SiteRef, hostnames []string) error {
	container, found, err := d.inspectContainer(ctx, ResourceName(ref.SiteID))
	if err != nil {
		return err
	}
	if !found {
		return errors.New("site container is missing")
	}
	if err := requireOwnership(container.Config.Labels, ownershipLabels(ref.AccountID, ref.SiteID, ref.NodeID)); err != nil {
		return err
	}
	if len(hostnames) == 0 {
		return d.deleteCaddyRoute(ctx, ref.SiteID, "")
	}
	hostnames, err = normalizeHostnames(hostnames)
	if err != nil {
		return err
	}
	hostPort, err := firstPublishedPort(container)
	if err != nil {
		return err
	}
	return d.upsertCaddyRouteHosts(ctx, ref.SiteID, hostnames, hostPort, false)
}

// CertificateStatus performs a hostname-validating TLS handshake against the
// public endpoint. With On-Demand TLS this also triggers allowlisted issuance.
func (d *Docker) CertificateStatus(
	ctx context.Context,
	hostname, ingressIPv4 string,
) (CertificateObservation, error) {
	if _, err := normalizeHostnames([]string{hostname}); err != nil {
		return CertificateObservation{}, err
	}
	if err := ValidatePublicIPv4(ingressIPv4); err != nil {
		return CertificateObservation{}, err
	}
	return d.certificateProbe(ctx, hostname, ingressIPv4)
}

func probeCertificate(ctx context.Context, hostname, ingressIPv4 string) (CertificateObservation, error) {
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := (&tls.Dialer{
		NetDialer: dialer,
		Config: &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: hostname,
		},
	}).DialContext(ctx, "tcp", net.JoinHostPort(ingressIPv4, "443"))
	if err != nil {
		return CertificateObservation{}, fmt.Errorf("TLS certificate not ready: %w", err)
	}
	defer func() { _ = conn.Close() }()
	tlsConn, ok := conn.(*tls.Conn)
	if !ok || len(tlsConn.ConnectionState().PeerCertificates) == 0 {
		return CertificateObservation{}, errors.New("TLS peer returned no certificate")
	}
	certificate := tlsConn.ConnectionState().PeerCertificates[0]
	if err := certificate.VerifyHostname(hostname); err != nil {
		return CertificateObservation{}, fmt.Errorf("TLS certificate hostname mismatch: %w", err)
	}
	return CertificateObservation{ExpiresAt: certificate.NotAfter.UTC()}, nil
}

func (d *Docker) validateSpec(spec SiteSpec) error {
	if spec.SiteID == uuid.Nil || spec.AccountID == uuid.Nil || spec.NodeID == uuid.Nil {
		return errors.New("site, account, and node IDs are required")
	}
	if spec.Image != d.allowedImage {
		return errors.New("site image is outside the curated allowlist")
	}
	if spec.InternalPort == 0 || spec.MemoryBytes < 64*1024*1024 || spec.NanoCPUs <= 0 {
		return errors.New("invalid site runtime limits")
	}
	if strings.TrimSpace(spec.Domain) == "" {
		return errors.New("site domain is required")
	}
	return nil
}

type dockerContainer struct {
	Config struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	State struct {
		Running bool `json:"Running"`
	} `json:"State"`
	NetworkSettings struct {
		Ports map[string][]portBinding `json:"Ports"`
	} `json:"NetworkSettings"`
}

type portBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

func (d *Docker) inspectContainer(ctx context.Context, name string) (*dockerContainer, bool, error) {
	var out dockerContainer
	status, err := d.engineRequestStatus(ctx, http.MethodGet, "/containers/"+url.PathEscape(name)+"/json", nil, &out)
	if status == http.StatusNotFound {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if status != http.StatusOK {
		return nil, false, unexpectedStatus(status)
	}
	return &out, true, nil
}

func (d *Docker) createContainer(
	ctx context.Context,
	name, networkName, volumeName string,
	labels map[string]string,
	spec SiteSpec,
) error {
	labels = cloneLabels(labels)
	labels["opencloud.domain"] = spec.Domain
	portKey := strconv.Itoa(int(spec.InternalPort)) + "/tcp"
	pidsLimit := int64(128)
	body := map[string]any{
		"Image":        spec.Image,
		"User":         "65532:65532",
		"Env":          []string{"OPENCLOUD_SITE_DOMAIN=" + spec.Domain},
		"Labels":       labels,
		"ExposedPorts": map[string]any{portKey: map[string]any{}},
		"HostConfig": map[string]any{
			"NetworkMode":    networkName,
			"ReadonlyRootfs": true,
			"CapDrop":        []string{"ALL"},
			"SecurityOpt":    []string{"no-new-privileges:true"},
			"Memory":         spec.MemoryBytes,
			"NanoCpus":       spec.NanoCPUs,
			"PidsLimit":      pidsLimit,
			"RestartPolicy":  map[string]any{"Name": "unless-stopped"},
			"PortBindings": map[string][]portBinding{
				portKey: {{HostIP: "127.0.0.1", HostPort: ""}},
			},
			"Mounts": []map[string]any{{
				"Type":     "volume",
				"Source":   volumeName,
				"Target":   "/srv",
				"ReadOnly": false,
			}},
			"Tmpfs": map[string]string{"/tmp": "rw,noexec,nosuid,size=16777216"},
		},
	}
	return d.engineRequest(ctx, http.MethodPost, "/containers/create?name="+url.QueryEscape(name), body, nil, http.StatusCreated)
}

func (d *Docker) startContainer(ctx context.Context, name string) error {
	return d.engineRequest(ctx, http.MethodPost, "/containers/"+url.PathEscape(name)+"/start", nil, nil, http.StatusNoContent, http.StatusNotModified)
}

func (d *Docker) ensureNetwork(ctx context.Context, name string, labels map[string]string) error {
	var inspect struct {
		Labels map[string]string `json:"Labels"`
	}
	status, err := d.engineRequestStatus(ctx, http.MethodGet, "/networks/"+url.PathEscape(name), nil, &inspect)
	if err == nil && status == http.StatusOK {
		return requireOwnership(inspect.Labels, labels)
	}
	if status != http.StatusNotFound {
		if err != nil {
			return err
		}
		return unexpectedStatus(status)
	}
	body := map[string]any{
		"Name":       name,
		"Driver":     "bridge",
		"Internal":   false,
		"Attachable": false,
		"Labels":     labels,
	}
	status, err = d.engineRequestStatus(ctx, http.MethodPost, "/networks/create", body, nil)
	if err != nil {
		return err
	}
	if status == http.StatusCreated {
		return nil
	}
	if status == http.StatusConflict {
		return d.ensureNetwork(ctx, name, labels)
	}
	return unexpectedStatus(status)
}

func (d *Docker) ensureVolume(ctx context.Context, name string, labels map[string]string) error {
	var inspect struct {
		Labels map[string]string `json:"Labels"`
	}
	status, err := d.engineRequestStatus(ctx, http.MethodGet, "/volumes/"+url.PathEscape(name), nil, &inspect)
	if err == nil && status == http.StatusOK {
		return requireOwnership(inspect.Labels, labels)
	}
	if status != http.StatusNotFound {
		if err != nil {
			return err
		}
		return unexpectedStatus(status)
	}
	body := map[string]any{"Name": name, "Driver": "local", "Labels": labels}
	if err := d.engineRequest(ctx, http.MethodPost, "/volumes/create", body, nil, http.StatusCreated); err != nil {
		return err
	}
	return d.ensureVolume(ctx, name, labels)
}

func (d *Docker) removeNetwork(ctx context.Context, name string, labels map[string]string) error {
	var inspect struct {
		Labels map[string]string `json:"Labels"`
	}
	status, err := d.engineRequestStatus(ctx, http.MethodGet, "/networks/"+url.PathEscape(name), nil, &inspect)
	if status == http.StatusNotFound {
		return nil
	}
	if err != nil || status != http.StatusOK {
		return errors.Join(err, unexpectedStatus(status))
	}
	if err := requireOwnership(inspect.Labels, labels); err != nil {
		return err
	}
	return d.engineRequest(ctx, http.MethodDelete, "/networks/"+url.PathEscape(name), nil, nil, http.StatusNoContent, http.StatusNotFound)
}

func (d *Docker) removeVolume(ctx context.Context, name string, labels map[string]string) error {
	var inspect struct {
		Labels map[string]string `json:"Labels"`
	}
	status, err := d.engineRequestStatus(ctx, http.MethodGet, "/volumes/"+url.PathEscape(name), nil, &inspect)
	if status == http.StatusNotFound {
		return nil
	}
	if err != nil || status != http.StatusOK {
		return errors.Join(err, unexpectedStatus(status))
	}
	if err := requireOwnership(inspect.Labels, labels); err != nil {
		return err
	}
	return d.engineRequest(ctx, http.MethodDelete, "/volumes/"+url.PathEscape(name), nil, nil, http.StatusNoContent, http.StatusNotFound)
}

func ownershipLabels(accountID, siteID, nodeID uuid.UUID) map[string]string {
	return map[string]string{
		managedLabelKey: "true",
		accountLabelKey: accountID.String(),
		siteLabelKey:    siteID.String(),
		nodeLabelKey:    nodeID.String(),
	}
}

func requireOwnership(actual, expected map[string]string) error {
	for key, value := range expected {
		if actual[key] != value {
			return fmt.Errorf("refusing to modify resource: ownership label %s mismatch", key)
		}
	}
	return nil
}

func cloneLabels(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels)+1)
	for key, value := range labels {
		out[key] = value
	}
	return out
}

func publishedPort(container *dockerContainer, internalPort uint16) (string, error) {
	bindings := container.NetworkSettings.Ports[strconv.Itoa(int(internalPort))+"/tcp"]
	if len(bindings) != 1 || bindings[0].HostIP != "127.0.0.1" || bindings[0].HostPort == "" {
		return "", errors.New("site container does not have exactly one loopback port binding")
	}
	return bindings[0].HostPort, nil
}

func firstPublishedPort(container *dockerContainer) (string, error) {
	if len(container.NetworkSettings.Ports) != 1 {
		return "", errors.New("site container does not have exactly one published port")
	}
	for _, bindings := range container.NetworkSettings.Ports {
		if len(bindings) != 1 || bindings[0].HostIP != "127.0.0.1" || bindings[0].HostPort == "" {
			return "", errors.New("site container port is not loopback-only")
		}
		return bindings[0].HostPort, nil
	}
	return "", errors.New("site container has no published port")
}

func (d *Docker) engineRequest(
	ctx context.Context,
	method, requestPath string,
	body any,
	out any,
	accepted ...int,
) error {
	status, err := d.engineRequestStatus(ctx, method, requestPath, body, out)
	if err != nil {
		return err
	}
	for _, candidate := range accepted {
		if status == candidate {
			return nil
		}
	}
	return unexpectedStatus(status)
}

func (d *Docker) engineRequestStatus(
	ctx context.Context,
	method, requestPath string,
	body any,
	out ...any,
) (int, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://docker/"+dockerAPIVersion+requestPath, reader)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := d.engine.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && len(out) > 0 && out[0] != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out[0]); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
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

func (d *Docker) upsertCaddyRoute(ctx context.Context, spec SiteSpec, hostPort string) error {
	return d.upsertCaddyRouteHosts(ctx, spec.SiteID, []string{spec.Domain}, hostPort, true)
}

func (d *Docker) upsertCaddyRouteHosts(
	ctx context.Context,
	siteID uuid.UUID,
	hostnames []string,
	hostPort string,
	preserveExisting bool,
) error {
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
		status, etag, err := d.caddyRequest(ctx, http.MethodGet, "/id/"+url.PathEscape(route.ID), nil, "", &existing)
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
			status, _, err = d.caddyRequest(ctx, http.MethodPatch, "/id/"+url.PathEscape(route.ID), route, etag, nil)
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

		routesPath := "/config/apps/http/servers/" + url.PathEscape(d.serverID) + "/routes"
		status, etag, err = d.caddyRequest(ctx, http.MethodGet, routesPath, nil, "", nil)
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return fmt.Errorf("caddy server routes unavailable: %w", unexpectedStatus(status))
		}
		status, _, err = d.caddyRequest(ctx, http.MethodPost, routesPath, route, etag, nil)
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

func (d *Docker) deleteCaddyRoute(ctx context.Context, siteID uuid.UUID, expectedDomain string) error {
	id := caddyRouteID(siteID)
	for attempt := 0; attempt < caddyRetries; attempt++ {
		var route caddyRoute
		status, etag, err := d.caddyRequest(ctx, http.MethodGet, "/id/"+url.PathEscape(id), nil, "", &route)
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
		status, _, err = d.caddyRequest(ctx, http.MethodDelete, "/id/"+url.PathEscape(id), nil, etag, nil)
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

func (d *Docker) caddyRequest(
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
	endpoint := *d.caddyURL
	endpoint.Path = strings.TrimRight(d.caddyURL.Path, "/") + requestPath
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
	resp, err := d.caddy.Do(req)
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
