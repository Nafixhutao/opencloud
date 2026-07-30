package provisioner

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"
)

// FakeDNS is an in-memory, concurrency-safe DNS provisioner for tests and
// non-production development. Verification checks internal state; DNS record
// operations are no-ops that record calls for assertions.
type FakeDNS struct {
	mu               sync.Mutex
	verifications    map[string]string // hostname -> token
	dnsRecords       map[string][]DNSRecord
	verified         map[string]bool
	failNext         error
}

// NewFakeDNS constructs an empty fake DNS adapter.
func NewFakeDNS() *FakeDNS {
	return &FakeDNS{
		verifications: make(map[string]string),
		dnsRecords:    make(map[string][]DNSRecord),
		verified:      make(map[string]bool),
	}
}

// FailNext makes the next operation fail. It is intended for tests.
func (f *FakeDNS) FailNext(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNext = err
}

func (f *FakeDNS) takeFailure() error {
	if f.failNext == nil {
		return nil
	}
	err := f.failNext
	f.failNext = nil
	return err
}

// GenerateVerification stores the token and returns TXT instructions.
func (f *FakeDNS) GenerateVerification(_ context.Context, domain DomainSpec) (VerificationInstructions, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.takeFailure(); err != nil {
		return VerificationInstructions{}, err
	}
	f.verifications[domain.Hostname] = domain.VerificationToken
	return VerificationInstructions{
		Type: "txt",
		Records: []DNSRecord{
			{
				Type:    "TXT",
				Name:    "_opencloud-verify." + domain.Hostname,
				Content: domain.VerificationToken,
				TTL:     300,
			},
		},
	}, nil
}

// VerifyOwnership checks the in-memory state.
func (f *FakeDNS) VerifyOwnership(_ context.Context, domain DomainSpec) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.takeFailure(); err != nil {
		return false, err
	}
	token, exists := f.verifications[domain.Hostname]
	if !exists || token != domain.VerificationToken {
		return false, nil
	}
	f.verified[domain.Hostname] = true
	return true, nil
}

// SetDNSRecords records the call for test assertions.
func (f *FakeDNS) SetDNSRecords(_ context.Context, zoneID string, records []DNSRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.takeFailure(); err != nil {
		return err
	}
	f.dnsRecords[zoneID] = append(f.dnsRecords[zoneID], records...)
	return nil
}

// DeleteDNSRecords clears the in-memory records.
func (f *FakeDNS) DeleteDNSRecords(_ context.Context, zoneID string, records []DNSRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.takeFailure(); err != nil {
		return err
	}
	delete(f.dnsRecords, zoneID)
	return nil
}

// IsVerified returns true when the domain was successfully verified.
func (f *FakeDNS) IsVerified(hostname string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.verified[hostname]
}

// SetVerificationToken is a test helper that pre-seeds a token.
func (f *FakeDNS) SetVerificationToken(hostname, token string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.verifications[hostname] = token
}

// DNSCount returns the number of records stored for a zone (test helper).
func (f *FakeDNS) DNSCount(zoneID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.dnsRecords[zoneID])
}

// ErrNotConfiguredFake is a sentinel used by tests to simulate a missing
// provider configuration.
var ErrNotConfiguredFake = errors.New("dns provider not configured")

// FakeDNSNotConfigured returns ErrNotConfigured on every operation.
type FakeDNSNotConfigured struct{}

func (FakeDNSNotConfigured) GenerateVerification(_ context.Context, _ DomainSpec) (VerificationInstructions, error) {
	return VerificationInstructions{}, ErrNotConfigured
}
func (FakeDNSNotConfigured) VerifyOwnership(_ context.Context, _ DomainSpec) (bool, error) {
	return false, ErrNotConfigured
}
func (FakeDNSNotConfigured) SetDNSRecords(_ context.Context, _ string, _ []DNSRecord) error {
	return ErrNotConfigured
}
func (FakeDNSNotConfigured) DeleteDNSRecords(_ context.Context, _ string, _ []DNSRecord) error {
	return ErrNotConfigured
}

// Compile-time interface checks.
var (
	_ DNSProvisioner = (*FakeDNS)(nil)
	_ DNSProvisioner = (*FakeDNSNotConfigured)(nil)
	_ DNSProvisioner = (*ManualDNS)(nil)
	_ DNSProvisioner = (*CloudflareDNS)(nil)
)

// Ensure uuid is used (prevents "imported and not used" if needed by callers
// that reference DomainSpec but not uuid directly).
var _ = uuid.Nil