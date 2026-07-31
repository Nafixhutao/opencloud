package provisioner

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Fake is an in-memory, concurrency-safe provisioner used by tests and
// non-production development. Repeating lifecycle calls converges.
type Fake struct {
	mu          sync.Mutex
	sites       map[uuid.UUID]fakeSite
	siteDomains map[uuid.UUID][]string
	failNext    error
}

type fakeSite struct {
	accountID uuid.UUID
	nodeID    uuid.UUID
	domain    string
	state     SiteState
}

// NewFake constructs an empty fake backend.
func NewFake() *Fake {
	return &Fake{
		sites:       make(map[uuid.UUID]fakeSite),
		siteDomains: make(map[uuid.UUID][]string),
	}
}

// FailNext makes the next backend operation fail. It is intended for tests.
func (f *Fake) FailNext(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNext = err
}

func (f *Fake) takeFailure() error {
	if f.failNext == nil {
		return nil
	}
	err := f.failNext
	f.failNext = nil
	return err
}

// CreateSite creates or converges an existing owned site to running.
func (f *Fake) CreateSite(_ context.Context, spec SiteSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.takeFailure(); err != nil {
		return err
	}
	if existing, ok := f.sites[spec.SiteID]; ok {
		if existing.accountID != spec.AccountID || existing.nodeID != spec.NodeID {
			return errors.New("site ownership mismatch")
		}
		existing.state = SiteStateRunning
		existing.domain = spec.Domain
		f.sites[spec.SiteID] = existing
		if len(f.siteDomains[spec.SiteID]) == 0 {
			f.siteDomains[spec.SiteID] = []string{spec.Domain}
		}
		return nil
	}
	f.sites[spec.SiteID] = fakeSite{
		accountID: spec.AccountID,
		nodeID:    spec.NodeID,
		domain:    spec.Domain,
		state:     SiteStateRunning,
	}
	f.siteDomains[spec.SiteID] = []string{spec.Domain}
	return nil
}

// DeleteSite removes an owned site; missing is success.
func (f *Fake) DeleteSite(_ context.Context, ref SiteRef) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.takeFailure(); err != nil {
		return err
	}
	existing, ok := f.sites[ref.SiteID]
	if !ok {
		return nil
	}
	if existing.accountID != ref.AccountID || existing.nodeID != ref.NodeID {
		return errors.New("site ownership mismatch")
	}
	delete(f.sites, ref.SiteID)
	delete(f.siteDomains, ref.SiteID)
	return nil
}

// SetSiteDomains converges the hostname set for an owned running site.
func (f *Fake) SetSiteDomains(_ context.Context, ref SiteRef, hostnames []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.takeFailure(); err != nil {
		return err
	}
	existing, ok := f.sites[ref.SiteID]
	if !ok {
		return errors.New("site missing")
	}
	if existing.accountID != ref.AccountID || existing.nodeID != ref.NodeID {
		return errors.New("site ownership mismatch")
	}
	copyOfHostnames := append([]string(nil), hostnames...)
	sort.Strings(copyOfHostnames)
	f.siteDomains[ref.SiteID] = copyOfHostnames
	return nil
}

// CertificateStatus reports a valid short-lived observation when a fake route
// contains hostname. This models Caddy after an on-demand handshake.
func (f *Fake) CertificateStatus(
	_ context.Context,
	hostname, _ string,
) (CertificateObservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.takeFailure(); err != nil {
		return CertificateObservation{}, err
	}
	for _, hostnames := range f.siteDomains {
		for _, candidate := range hostnames {
			if candidate == hostname {
				return CertificateObservation{ExpiresAt: time.Now().UTC().Add(90 * 24 * time.Hour)}, nil
			}
		}
	}
	return CertificateObservation{}, errors.New("certificate not ready")
}

// SiteDomains returns a defensive copy for tests.
func (f *Fake) SiteDomains(siteID uuid.UUID) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.siteDomains[siteID]...)
}

// SuspendSite stops an owned site while retaining it.
func (f *Fake) SuspendSite(_ context.Context, ref SiteRef) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.takeFailure(); err != nil {
		return err
	}
	existing, ok := f.sites[ref.SiteID]
	if !ok {
		return errors.New("site missing")
	}
	if existing.accountID != ref.AccountID || existing.nodeID != ref.NodeID {
		return errors.New("site ownership mismatch")
	}
	existing.state = SiteStateSuspended
	f.sites[ref.SiteID] = existing
	delete(f.siteDomains, ref.SiteID)
	return nil
}

// ResumeSite starts an owned site.
func (f *Fake) ResumeSite(_ context.Context, ref SiteRef) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.takeFailure(); err != nil {
		return err
	}
	existing, ok := f.sites[ref.SiteID]
	if !ok {
		return errors.New("site missing")
	}
	if existing.accountID != ref.AccountID || existing.nodeID != ref.NodeID {
		return errors.New("site ownership mismatch")
	}
	existing.state = SiteStateRunning
	f.sites[ref.SiteID] = existing
	f.siteDomains[ref.SiteID] = []string{existing.domain}
	return nil
}

// SiteStatus returns the observed state without adopting unknown objects.
func (f *Fake) SiteStatus(_ context.Context, ref SiteRef) (SiteState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, ok := f.sites[ref.SiteID]
	if !ok {
		return SiteStateMissing, nil
	}
	if existing.accountID != ref.AccountID || existing.nodeID != ref.NodeID {
		return "", errors.New("site ownership mismatch")
	}
	return existing.state, nil
}
