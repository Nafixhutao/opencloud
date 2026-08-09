package deployment

import (
	"context"
	"fmt"
	"sync"
)

const (
	ActionStart         = "start"
	ActionCheckHealth   = "health"
	ActionSwitchTraffic = "switch_caddy_traffic"
	ActionRetire        = "retire"
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

func (f *FakeRuntime) Start(_ context.Context, revision Revision) error {
	if err := ValidateProviderRevision(revision); err != nil {
		return err
	}
	f.append(Action{Name: ActionStart, Deployment: revision})
	return f.fail(ActionStart)
}

func (f *FakeRuntime) CheckHealth(_ context.Context, revision Revision) error {
	if err := ValidateProviderRevision(revision); err != nil {
		return err
	}
	f.append(Action{Name: ActionCheckHealth, Deployment: revision})
	return f.fail(ActionCheckHealth)
}

func (f *FakeRuntime) SwitchCaddyTraffic(_ context.Context, traffic TrafficSwitch) error {
	if err := traffic.Validate(); err != nil {
		return err
	}
	f.append(Action{Name: ActionSwitchTraffic, Deployment: traffic.Target, Switch: &traffic})
	return f.fail(ActionSwitchTraffic)
}

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
