package provisioner

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ErrDNSNotReady is retryable and means the expected public record has not
// propagated yet.
var ErrDNSNotReady = errors.New("dns records are not ready")

// DomainDNSProvisioner owns DNS verification and optional record automation.
// Implementations must converge when a durable job repeats a request.
type DomainDNSProvisioner interface {
	Instructions(context.Context, DomainDNSSpec) (DomainInstructions, error)
	VerifyOwnership(context.Context, DomainDNSSpec) (bool, error)
	VerifyRouting(context.Context, DomainDNSSpec) (bool, error)
	EnsureRecords(context.Context, DomainDNSSpec) ([]string, error)
	DeleteRecords(context.Context, DomainDNSSpec, []string) error
}

// DomainDNSSpec contains only provider-neutral, validated values.
type DomainDNSSpec struct {
	DomainID          uuid.UUID
	AccountID         uuid.UUID
	Hostname          string
	VerificationToken string
	IngressIPv4       string
	Provider          string
	ZoneID            string
}

// DomainInstructions is safe to return to the owning customer.
type DomainInstructions struct {
	VerificationExpiresAt time.Time   `json:"verification_expires_at"`
	Records               []DNSRecord `json:"records"`
}

// DNSRecord is one customer-visible or provider-managed record.
type DNSRecord struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
}

// ManualDNS implements the universal bring-your-own-DNS flow. It never writes
// provider state; customers apply the returned records themselves.
type ManualDNS struct {
	resolver *net.Resolver
}

// NewManualDNS constructs a resolver that uses the configured DNS server.
func NewManualDNS(resolverAddress string) (*ManualDNS, error) {
	resolverAddress = strings.TrimSpace(resolverAddress)
	if resolverAddress == "" {
		resolverAddress = "1.1.1.1:53"
	}
	host, port, err := net.SplitHostPort(resolverAddress)
	portNumber, portErr := strconv.Atoi(port)
	if err != nil || net.ParseIP(host) == nil || portErr != nil || portNumber < 1 || portNumber > 65535 {
		return nil, errors.New("DOMAIN_DNS_RESOLVER must be an IP:port")
	}
	return &ManualDNS{resolver: &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			dialer := net.Dialer{Timeout: 5 * time.Second}
			return dialer.DialContext(ctx, network, resolverAddress)
		},
	}}, nil
}

// Instructions returns only the TXT ownership challenge. The service exposes
// the ingress A record only after ownership proof has been consumed.
func (m *ManualDNS) Instructions(_ context.Context, spec DomainDNSSpec) (DomainInstructions, error) {
	if spec.VerificationToken == "" {
		return DomainInstructions{}, errors.New("verification token is required")
	}
	if net.ParseIP(spec.IngressIPv4) == nil {
		return DomainInstructions{}, errors.New("ingress IPv4 address is required")
	}
	return DomainInstructions{Records: []DNSRecord{
		{Type: "TXT", Name: verificationRecordName(spec.Hostname), Content: spec.VerificationToken, TTL: 300},
	}}, nil
}

// VerifyOwnership checks the public TXT answer without exposing existence of
// any other tenant's domain record.
func (m *ManualDNS) VerifyOwnership(ctx context.Context, spec DomainDNSSpec) (bool, error) {
	records, err := m.resolver.LookupTXT(ctx, verificationRecordName(spec.Hostname))
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return false, nil
		}
		return false, fmt.Errorf("lookup ownership TXT: %w", err)
	}
	for _, record := range records {
		if record == spec.VerificationToken {
			return true, nil
		}
	}
	return false, nil
}

// VerifyRouting requires the hostname to resolve directly to the configured
// VPS address. Customers may enable an HTTP proxy after initial activation.
func (m *ManualDNS) VerifyRouting(ctx context.Context, spec DomainDNSSpec) (bool, error) {
	addresses, err := m.resolver.LookupIPAddr(ctx, spec.Hostname)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return false, nil
		}
		return false, fmt.Errorf("lookup ingress address: %w", err)
	}
	expected := net.ParseIP(spec.IngressIPv4)
	for _, address := range addresses {
		if address.IP.Equal(expected) {
			return true, nil
		}
	}
	return false, nil
}

// EnsureRecords is a no-op for customer-managed DNS.
func (*ManualDNS) EnsureRecords(context.Context, DomainDNSSpec) ([]string, error) {
	return nil, nil
}

// DeleteRecords is a no-op for customer-managed DNS.
func (*ManualDNS) DeleteRecords(context.Context, DomainDNSSpec, []string) error {
	return nil
}

func verificationRecordName(hostname string) string {
	return "_opencloud-verification." + strings.TrimSuffix(hostname, ".")
}

// ValidatePublicIPv4 rejects addresses that cannot be a public direct-ingress
// target for customer DNS.
func ValidatePublicIPv4(value string) error {
	address, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil || !address.Is4() || !address.IsGlobalUnicast() ||
		address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
		return errors.New("DOMAIN_INGRESS_IPV4 must be a public IPv4 address")
	}
	reserved := [...]netip.Prefix{
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("240.0.0.0/4"),
	}
	for _, prefix := range reserved {
		if prefix.Contains(address) {
			return errors.New("DOMAIN_INGRESS_IPV4 must be a public IPv4 address")
		}
	}
	return nil
}

// FakeDomainDNS is concurrency-safe and exposes explicit propagation controls
// so tests do not pass merely because instructions were generated.
type FakeDomainDNS struct {
	mu                   sync.Mutex
	ownership            map[string]map[string]struct{}
	routing              map[string]bool
	records              map[uuid.UUID][]string
	verifyOwnershipCalls int
	verifyRoutingCalls   int
	ensureRecordsCalls   int
	deleteRecordsCalls   int
	failNext             error
}

// NewFakeDomainDNS constructs an empty fake provider.
func NewFakeDomainDNS() *FakeDomainDNS {
	return &FakeDomainDNS{
		ownership: make(map[string]map[string]struct{}),
		routing:   make(map[string]bool),
		records:   make(map[uuid.UUID][]string),
	}
}

// Instructions returns deterministic manual DNS records for tests.
func (f *FakeDomainDNS) Instructions(_ context.Context, spec DomainDNSSpec) (DomainInstructions, error) {
	if err := f.takeFailure(); err != nil {
		return DomainInstructions{}, err
	}
	return DomainInstructions{Records: []DNSRecord{
		{Type: "TXT", Name: verificationRecordName(spec.Hostname), Content: spec.VerificationToken, TTL: 300},
	}}, nil
}

// VerifyOwnership reports whether the test supplied the expected TXT token.
func (f *FakeDomainDNS) VerifyOwnership(_ context.Context, spec DomainDNSSpec) (bool, error) {
	f.mu.Lock()
	f.verifyOwnershipCalls++
	f.mu.Unlock()
	if err := f.takeFailure(); err != nil {
		return false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	_, verified := f.ownership[spec.Hostname][spec.VerificationToken]
	return verified, nil
}

// VerifyRouting reports whether the test marked the hostname as routed.
func (f *FakeDomainDNS) VerifyRouting(_ context.Context, spec DomainDNSSpec) (bool, error) {
	f.mu.Lock()
	f.verifyRoutingCalls++
	f.mu.Unlock()
	if err := f.takeFailure(); err != nil {
		return false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.routing[spec.Hostname], nil
}

// EnsureRecords records a deterministic provider record identifier for tests.
func (f *FakeDomainDNS) EnsureRecords(_ context.Context, spec DomainDNSSpec) ([]string, error) {
	f.mu.Lock()
	f.ensureRecordsCalls++
	f.mu.Unlock()
	if err := f.takeFailure(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := []string{"record-" + spec.DomainID.String()}
	f.records[spec.DomainID] = ids
	return append([]string(nil), ids...), nil
}

// DeleteRecords removes records previously created by the fake provider.
func (f *FakeDomainDNS) DeleteRecords(_ context.Context, spec DomainDNSSpec, _ []string) error {
	f.mu.Lock()
	f.deleteRecordsCalls++
	f.mu.Unlock()
	if err := f.takeFailure(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.records, spec.DomainID)
	return nil
}

// DomainDNSCallCounts is a concurrency-safe snapshot for behavioral tests.
type DomainDNSCallCounts struct {
	VerifyOwnership int
	VerifyRouting   int
	EnsureRecords   int
	DeleteRecords   int
}

// CallCounts reports provider operations without exposing fake internals.
func (f *FakeDomainDNS) CallCounts() DomainDNSCallCounts {
	f.mu.Lock()
	defer f.mu.Unlock()
	return DomainDNSCallCounts{
		VerifyOwnership: f.verifyOwnershipCalls,
		VerifyRouting:   f.verifyRoutingCalls,
		EnsureRecords:   f.ensureRecordsCalls,
		DeleteRecords:   f.deleteRecordsCalls,
	}
}

// SetOwnership simulates publishing an additional TXT challenge value. Public
// DNS may contain concurrent ownership tokens for the same hostname.
func (f *FakeDomainDNS) SetOwnership(hostname, token string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ownership[hostname] == nil {
		f.ownership[hostname] = make(map[string]struct{})
	}
	f.ownership[hostname][token] = struct{}{}
}

// SetRouting simulates public A-record propagation.
func (f *FakeDomainDNS) SetRouting(hostname string, ready bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routing[hostname] = ready
}

// FailNext injects one provider failure.
func (f *FakeDomainDNS) FailNext(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNext = err
}

func (f *FakeDomainDNS) takeFailure() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	err := f.failNext
	f.failNext = nil
	return err
}

// RecordIDs returns stable sorted IDs for test assertions.
func (f *FakeDomainDNS) RecordIDs(domainID uuid.UUID) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := append([]string(nil), f.records[domainID]...)
	sort.Strings(ids)
	return ids
}
