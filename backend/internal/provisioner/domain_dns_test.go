package provisioner

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestFakeDomainDNSRequiresExplicitOwnershipAndRouting(t *testing.T) {
	dns := NewFakeDomainDNS()
	spec := DomainDNSSpec{
		DomainID: uuid.New(), AccountID: uuid.New(), Hostname: "www.example.com",
		VerificationToken: "oc_verify_test", IngressIPv4: "192.0.2.10",
	}

	instructions, err := dns.Instructions(context.Background(), spec)
	require.NoError(t, err)
	require.Equal(t, []DNSRecord{{
		Type: "TXT", Name: "_opencloud-verification.www.example.com",
		Content: spec.VerificationToken, TTL: 300,
	}}, instructions.Records)
	verified, err := dns.VerifyOwnership(context.Background(), spec)
	require.NoError(t, err)
	require.False(t, verified)
	routed, err := dns.VerifyRouting(context.Background(), spec)
	require.NoError(t, err)
	require.False(t, routed)

	dns.SetOwnership(spec.Hostname, spec.VerificationToken)
	dns.SetRouting(spec.Hostname, true)
	verified, err = dns.VerifyOwnership(context.Background(), spec)
	require.NoError(t, err)
	require.True(t, verified)
	routed, err = dns.VerifyRouting(context.Background(), spec)
	require.NoError(t, err)
	require.True(t, routed)
}

func TestManualAndFakeDNSUseExactVerificationRecordName(t *testing.T) {
	spec := DomainDNSSpec{
		DomainID: uuid.New(), AccountID: uuid.New(), Hostname: "www.example.com.",
		VerificationToken: "oc_verify_test", IngressIPv4: "8.8.8.8",
	}
	expected := "_opencloud-verification.www.example.com"

	manual, err := NewManualDNS("1.1.1.1:53")
	require.NoError(t, err)
	manualInstructions, err := manual.Instructions(context.Background(), spec)
	require.NoError(t, err)
	require.Equal(t, expected, manualInstructions.Records[0].Name)

	fakeInstructions, err := NewFakeDomainDNS().Instructions(context.Background(), spec)
	require.NoError(t, err)
	require.Equal(t, expected, fakeInstructions.Records[0].Name,
		"the fake must expose the same customer DNS contract as the real provider")
}

func TestNewManualDNSRejectsUnsafeResolver(t *testing.T) {
	for _, resolver := range []string{
		"resolver.example.com:53",
		"1.1.1.1",
		"1.1.1.1:not-a-port",
		"1.1.1.1:0",
		"1.1.1.1:65536",
	} {
		_, err := NewManualDNS(resolver)
		require.Error(t, err, resolver)
	}
	_, err := NewManualDNS("1.1.1.1:53")
	require.NoError(t, err)
}

func TestValidatePublicIPv4RejectsSpecialUseRanges(t *testing.T) {
	require.NoError(t, ValidatePublicIPv4("8.8.8.8"))
	for _, address := range []string{
		"not-an-ip",
		"2001:4860:4860::8888",
		"10.0.0.1",
		"100.64.0.1",
		"127.0.0.1",
		"169.254.1.1",
		"192.0.2.1",
		"198.18.0.1",
		"198.51.100.1",
		"203.0.113.1",
		"240.0.0.1",
	} {
		require.Error(t, ValidatePublicIPv4(address), address)
	}
}
