package provisioner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DNSProvisioner manages domain verification and DNS record operations behind a
// provider-neutral boundary. Every adapter must be safe to retry.
type DNSProvisioner interface {
	// GenerateVerification produces a one-time token and instructions the
	// customer must apply to their DNS zone.
	GenerateVerification(ctx context.Context, domain DomainSpec) (VerificationInstructions, error)

	// VerifyOwnership checks the DNS TXT record for the verification token.
	// Returns true when the record is present and matches.
	VerifyOwnership(ctx context.Context, domain DomainSpec) (bool, error)

	// SetDNSRecords creates or updates DNS records for a verified domain.
	SetDNSRecords(ctx context.Context, zoneID string, records []DNSRecord) error

	// DeleteDNSRecords removes DNS records.
	DeleteDNSRecords(ctx context.Context, zoneID string, records []DNSRecord) error
}

// DomainSpec is the provider-neutral input for domain operations.
type DomainSpec struct {
	DomainID          uuid.UUID
	AccountID         uuid.UUID
	Hostname          string
	VerificationToken string
	DNSProvider       string
	DNSZoneID         *string
}

// VerificationInstructions tells the customer what to configure.
type VerificationInstructions struct {
	Type    string      `json:"type"`    // "txt"
	Records []DNSRecord `json:"records"` // at least one TXT record
}

// DNSRecord represents a single DNS resource record.
type DNSRecord struct {
	Type    string `json:"type"`    // TXT, A, CNAME
	Name    string `json:"name"`    // relative name, e.g. "@" or "_opencloud-verify"
	Content string `json:"content"` // record value
	TTL     int    `json:"ttl"`     // seconds
}

// ErrNotConfigured is returned when a DNS provider is selected but not
// configured (e.g. Cloudflare enabled without credentials).
var ErrNotConfigured = errors.New("dns provider is not configured")

// NewToken generates a random 32-byte hex verification token.
func NewToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand.Read failed: %v", err))
	}
	return "opencloud-verify=" + hex.EncodeToString(b)
}

// ManualDNS is the default universal adapter. It produces TXT instructions
// for the customer to apply and verifies ownership by querying public DNS.
type ManualDNS struct {
	resolver string
}

// NewManualDNS constructs a ManualDNS adapter.
func NewManualDNS(resolver string) *ManualDNS {
	if resolver == "" {
		resolver = "8.8.8.8:53"
	}
	return &ManualDNS{resolver: resolver}
}

// GenerateVerification produces TXT record instructions the customer must add
// to their DNS zone. The token is generated once and stored in the domain row.
func (m *ManualDNS) GenerateVerification(_ context.Context, domain DomainSpec) (VerificationInstructions, error) {
	token := domain.VerificationToken
	if token == "" {
		return VerificationInstructions{}, errors.New("verification token is required")
	}
	return VerificationInstructions{
		Type: "txt",
		Records: []DNSRecord{
			{
				Type:    "TXT",
				Name:    "_opencloud-verify." + domain.Hostname,
				Content: token,
				TTL:     300,
			},
			{
				Type:    "A",
				Name:    domain.Hostname,
				Content: "<your VPS public IP>",
				TTL:     300,
			},
		},
	}, nil
}

// VerifyOwnership checks the public DNS for the verification TXT record.
func (m *ManualDNS) VerifyOwnership(ctx context.Context, domain DomainSpec) (bool, error) {
	txtName := "_opencloud-verify." + strings.TrimSuffix(domain.Hostname, ".") + "."

	records, err := m.lookupTXT(ctx, txtName)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return false, nil
		}
		return false, fmt.Errorf("dns lookup %q: %w", txtName, err)
	}

	expected := domain.VerificationToken
	for _, record := range records {
		if record == expected {
			return true, nil
		}
	}
	return false, nil
}

// SetDNSRecords is a no-op for manual DNS — the customer manages their own.
func (m *ManualDNS) SetDNSRecords(_ context.Context, _ string, _ []DNSRecord) error {
	return nil
}

// DeleteDNSRecords is a no-op for manual DNS.
func (m *ManualDNS) DeleteDNSRecords(_ context.Context, _ string, _ []DNSRecord) error {
	return nil
}

func (m *ManualDNS) lookupTXT(ctx context.Context, name string) ([]string, error) {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, "udp", m.resolver)
		},
	}
	return resolver.LookupTXT(ctx, name)
}