package deployment

import (
	"context"
	"fmt"
	"sync"
)

const (
	// ActionStart records a runtime start attempt.
	ActionStart = "start"
	// ActionCheckHealth records a runtime health check.
	ActionCheckHealth = "health"
	// ActionSwitchTraffic records a Caddy traffic-switch attempt.
	ActionSwitchTraffic = "switch_caddy_traffic"
	// ActionRetire records retiring a previous runtime revision.
	ActionRetire = "retire"
)

// Action records a fake runtime call for sequence tests.
type Action struct {
	Name       string
	Deployment Revision
	Switch     *TrafficSwitch
}

// FakeRuntime is a deterministic runtime provider for tests. It never starts
// a process, accesses Caddy, or opens a network connection.
type FakeRuntime struct {
	FailAt string

	mu      sync.Mutex
	actions []Action
}

// Start validates and records a simulated runtime start.
func (f *FakeRuntime) Start(_ context.Context, revision Revision) error {
	if err := ValidateProviderRevision(revision); err != nil {
		return err
	}
	f.append(Action{Name: ActionStart, Deployment: revision})
	return f.fail(ActionStart)
}

// CheckHealth validates and records a simulated runtime health check.
func (f *FakeRuntime) CheckHealth(_ context.Context, revision Revision) error {
	if err := ValidateProviderRevision(revision); err != nil {
		return err
	}
	f.append(Action{Name: ActionCheckHealth, Deployment: revision})
	return f.fail(ActionCheckHealth)
}

// SwitchCaddyTraffic validates and records a simulated traffic switch.
func (f *FakeRuntime) SwitchCaddyTraffic(_ context.Context, traffic TrafficSwitch) error {
	if err := traffic.Validate(); err != nil {
		return err
	}
	f.append(Action{Name: ActionSwitchTraffic, Deployment: traffic.Target, Switch: &traffic})
	return f.fail(ActionSwitchTraffic)
}

// Retire validates and records a simulated runtime retirement.
func (f *FakeRuntime) Retire(_ context.Context, revision Revision) error {
	if err := ValidateProviderRevision(revision); err != nil {
		return err
	}
	f.append(Action{Name: ActionRetire, Deployment: revision})
	return f.fail(ActionRetire)
}

// Actions returns a copy of the observed calls in order.
func (f *FakeRuntime) Actions() []Action {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Action(nil), f.actions...)
}

func (f *FakeRuntime) append(action Action) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actions = append(f.actions, action)
}

func (f *FakeRuntime) fail(action string) error {
	if f.FailAt == action {
		return fmt.Errorf("fake runtime %s failure", action)
	}
	return nil
}
