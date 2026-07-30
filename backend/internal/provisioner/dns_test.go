package provisioner_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nazxf/opencloud/backend/internal/provisioner"
)

func TestFakeDNS_GenerateAndVerify(t *testing.T) {
	dns := provisioner.NewFakeDNS()
	ctx := context.Background()
	token := provisioner.NewToken()
	hostname := "www.example.com"

	spec := provisioner.DomainSpec{
		DomainID:          uuid.New(),
		AccountID:         uuid.New(),
		Hostname:          hostname,
		VerificationToken: token,
	}

	instructions, err := dns.GenerateVerification(ctx, spec)
	require.NoError(t, err)
	require.Equal(t, "txt", instructions.Type)
	require.Len(t, instructions.Records, 1)
	require.Equal(t, token, instructions.Records[0].Content)

	verified, err := dns.VerifyOwnership(ctx, spec)
	require.NoError(t, err)
	require.True(t, verified, "ownership should be verified after generating token")
}

func TestFakeDNS_VerifyWithoutGenerate(t *testing.T) {
	dns := provisioner.NewFakeDNS()
	ctx := context.Background()

	spec := provisioner.DomainSpec{
		DomainID:          uuid.New(),
		AccountID:         uuid.New(),
		Hostname:          "www.example.com",
		VerificationToken: "some-token",
	}

	verified, err := dns.VerifyOwnership(ctx, spec)
	require.NoError(t, err)
	require.False(t, verified, "should not verify without prior GenerateVerification")
}

func TestFakeDNS_VerifyWrongToken(t *testing.T) {
	dns := provisioner.NewFakeDNS()
	ctx := context.Background()
	hostname := "www.example.com"

	dns.SetVerificationToken(hostname, "correct-token")

	spec := provisioner.DomainSpec{
		DomainID:          uuid.New(),
		AccountID:         uuid.New(),
		Hostname:          hostname,
		VerificationToken: "wrong-token",
	}

	verified, err := dns.VerifyOwnership(ctx, spec)
	require.NoError(t, err)
	require.False(t, verified)
}

func TestFakeDNS_VerifyDifferentHostname(t *testing.T) {
	dns := provisioner.NewFakeDNS()
	ctx := context.Background()

	spec1 := provisioner.DomainSpec{
		DomainID:          uuid.New(),
		AccountID:         uuid.New(),
		Hostname:          "host-a.example.com",
		VerificationToken: provisioner.NewToken(),
	}
	_, err := dns.GenerateVerification(ctx, spec1)
	require.NoError(t, err)

	spec2 := provisioner.DomainSpec{
		DomainID:          uuid.New(),
		AccountID:         uuid.New(),
		Hostname:          "host-b.example.com",
		VerificationToken: spec1.VerificationToken,
	}
	verified, err := dns.VerifyOwnership(ctx, spec2)
	require.NoError(t, err)
	require.False(t, verified, "different hostname should not match")
}

func TestFakeDNS_FailNext(t *testing.T) {
	dns := provisioner.NewFakeDNS()
	dns.FailNext(provisioner.ErrNotConfiguredFake)
	ctx := context.Background()

	spec := provisioner.DomainSpec{
		DomainID:          uuid.New(),
		AccountID:         uuid.New(),
		Hostname:          "www.example.com",
		VerificationToken: provisioner.NewToken(),
	}

	_, err := dns.GenerateVerification(ctx, spec)
	require.Error(t, err)
	require.Equal(t, provisioner.ErrNotConfiguredFake, err)

	// Next call should succeed.
	instructions, err := dns.GenerateVerification(ctx, spec)
	require.NoError(t, err)
	require.NotNil(t, instructions)
}

func TestFakeDNS_SetDNSRecords(t *testing.T) {
	dns := provisioner.NewFakeDNS()
	ctx := context.Background()

	records := []provisioner.DNSRecord{
		{Type: "A", Name: "www.example.com", Content: "1.2.3.4", TTL: 300},
		{Type: "CNAME", Name: "www.example.com", Content: "example.com", TTL: 300},
	}

	err := dns.SetDNSRecords(ctx, "zone-123", records)
	require.NoError(t, err)
	require.Equal(t, 2, dns.DNSCount("zone-123"))

	err = dns.DeleteDNSRecords(ctx, "zone-123", records)
	require.NoError(t, err)
	require.Equal(t, 0, dns.DNSCount("zone-123"))
}

func TestFakeDNSNotConfigured_AlwaysErrors(t *testing.T) {
	dns := provisioner.FakeDNSNotConfigured{}
	ctx := context.Background()
	spec := provisioner.DomainSpec{
		DomainID:          uuid.New(),
		AccountID:         uuid.New(),
		Hostname:          "www.example.com",
		VerificationToken: "token",
	}

	_, err := dns.GenerateVerification(ctx, spec)
	require.ErrorIs(t, err, provisioner.ErrNotConfigured)

	_, err = dns.VerifyOwnership(ctx, spec)
	require.ErrorIs(t, err, provisioner.ErrNotConfigured)

	err = dns.SetDNSRecords(ctx, "z", nil)
	require.ErrorIs(t, err, provisioner.ErrNotConfigured)

	err = dns.DeleteDNSRecords(ctx, "z", nil)
	require.ErrorIs(t, err, provisioner.ErrNotConfigured)
}

func TestManualDNS_GenerateVerification(t *testing.T) {
	dns := provisioner.NewManualDNS("8.8.8.8:53")
	ctx := context.Background()
	token := provisioner.NewToken()

	spec := provisioner.DomainSpec{
		DomainID:          uuid.New(),
		AccountID:         uuid.New(),
		Hostname:          "www.example.com",
		VerificationToken: token,
	}

	instructions, err := dns.GenerateVerification(ctx, spec)
	require.NoError(t, err)
	require.Equal(t, "txt", instructions.Type)
	require.Len(t, instructions.Records, 2)

	var foundTXT, foundA bool
	for _, r := range instructions.Records {
		if r.Type == "TXT" && r.Content == token {
			foundTXT = true
		}
		if r.Type == "A" {
			foundA = true
		}
	}
	require.True(t, foundTXT, "should contain the TXT verification record")
	require.True(t, foundA, "should contain an A record instruction")
}

func TestManualDNS_VerifyRealDNS(t *testing.T) {
	dns := provisioner.NewManualDNS("8.8.8.8:53")
	ctx := context.Background()

	// This test checks a domain we know doesn't exist — it should return false, not error.
	spec := provisioner.DomainSpec{
		DomainID:          uuid.New(),
		AccountID:         uuid.New(),
		Hostname:          "this-domain-definitely-does-not-exist-29284.example",
		VerificationToken: "irrelevant-token",
	}

	verified, err := dns.VerifyOwnership(ctx, spec)
	require.NoError(t, err, "nonexistent domain should not cause an error")
	require.False(t, verified, "nonexistent domain should not pass verification")
}

func TestManualDNS_GenerateVerificationEmptyToken(t *testing.T) {
	dns := provisioner.NewManualDNS("")
	ctx := context.Background()

	spec := provisioner.DomainSpec{
		DomainID:          uuid.New(),
		AccountID:         uuid.New(),
		Hostname:          "www.example.com",
		VerificationToken: "",
	}

	_, err := dns.GenerateVerification(ctx, spec)
	require.Error(t, err, "empty token should be rejected")
}

func TestNewToken_Uniqueness(t *testing.T) {
	tokens := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token := provisioner.NewToken()
		require.False(t, tokens[token], "duplicate token generated: %s", token)
		tokens[token] = true
		require.Contains(t, token, "opencloud-verify=")
	}
}