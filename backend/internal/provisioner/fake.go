package provisioner

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"
)

// Fake is an in-memory, concurrency-safe provisioner used by tests and
// non-production development. Repeating lifecycle calls converges.
type Fake struct {
	mu       sync.Mutex
	sites    map[uuid.UUID]fakeSite
	failNext error
}

type fakeSite struct {
	accountID uuid.UUID
	nodeID    uuid.UUID
	state     SiteState
}

// NewFake constructs an empty fake backend.
func NewFake() *Fake {
	return &Fake{sites: make(map[uuid.UUID]fakeSite)}
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
		f.sites[spec.SiteID] = existing
		return nil
	}
	f.sites[spec.SiteID] = fakeSite{
		accountID: spec.AccountID,
		nodeID:    spec.NodeID,
		state:     SiteStateRunning,
	}
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
	return nil
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
