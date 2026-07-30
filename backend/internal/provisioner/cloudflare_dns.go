package provisioner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// CloudflareDNS manages DNS records through the Cloudflare API. It is
// feature-flagged and requires a scoped API token. Without credentials,
// every call returns ErrNotConfigured.
type CloudflareDNS struct {
	token  string
	client *http.Client
}

// NewCloudflareDNS validates and constructs the adapter. An empty token is
// accepted so the adapter can surface ErrNotConfigured at call time rather
// than crashing the worker on startup.
func NewCloudflareDNS(token string) *CloudflareDNS {
	return &CloudflareDNS{
		token: strings.TrimSpace(token),
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// GenerateVerification produces TXT instructions. For Cloudflare, the
// token is the same; auto-creation of the TXT record is not attempted
// until the customer grants zone access (future OAuth flow).
func (c *CloudflareDNS) GenerateVerification(_ context.Context, domain DomainSpec) (VerificationInstructions, error) {
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
		},
	}, nil
}

// VerifyOwnership checks the verification TXT record via the Cloudflare API.
// If the token is missing, ErrNotConfigured is returned.
func (c *CloudflareDNS) VerifyOwnership(ctx context.Context, domain DomainSpec) (bool, error) {
	if c.token == "" {
		return false, ErrNotConfigured
	}
	zoneID := ""
	if domain.DNSZoneID != nil {
		zoneID = *domain.DNSZoneID
	}
	if zoneID == "" {
		return false, errors.New("cloudflare zone ID is required for verification")
	}
	txtName := "_opencloud-verify." + domain.Hostname

	records, err := c.listDNSRecords(ctx, zoneID, "TXT", txtName)
	if err != nil {
		return false, fmt.Errorf("list cloudflare dns records: %w", err)
	}

	expected := domain.VerificationToken
	for _, record := range records {
		if record.Content == expected {
			return true, nil
		}
	}
	return false, nil
}

// SetDNSRecords creates or updates DNS records in the Cloudflare zone.
func (c *CloudflareDNS) SetDNSRecords(ctx context.Context, zoneID string, records []DNSRecord) error {
	if c.token == "" {
		return ErrNotConfigured
	}
	for _, record := range records {
		if err := c.createDNSRecord(ctx, zoneID, record); err != nil {
			return fmt.Errorf("create record %s %s: %w", record.Type, record.Name, err)
		}
	}
	return nil
}

// DeleteDNSRecords removes matching DNS records from the Cloudflare zone.
func (c *CloudflareDNS) DeleteDNSRecords(ctx context.Context, zoneID string, records []DNSRecord) error {
	if c.token == "" {
		return ErrNotConfigured
	}
	for _, record := range records {
		if err := c.deleteDNSRecord(ctx, zoneID, record); err != nil {
			return fmt.Errorf("delete record %s %s: %w", record.Type, record.Name, err)
		}
	}
	return nil
}

type cloudflareDNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
}

type cloudflareListResponse struct {
	Result []cloudflareDNSRecord `json:"result"`
	Errors []cloudflareAPIError  `json:"errors"`
}

type cloudflareAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *CloudflareDNS) listDNSRecords(ctx context.Context, zoneID, recordType, name string) ([]cloudflareDNSRecord, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?type=%s&name=%s",
		zoneID, recordType, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("cloudflare api auth failed (status %d): token may be revoked or expired", resp.StatusCode)
	}

	var list cloudflareListResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&list); err != nil {
		return nil, err
	}
	if len(list.Errors) > 0 {
		return nil, fmt.Errorf("cloudflare api error: [%d] %s", list.Errors[0].Code, list.Errors[0].Message)
	}
	return list.Result, nil
}

func (c *CloudflareDNS) createDNSRecord(ctx context.Context, zoneID string, record DNSRecord) error {
	body := map[string]any{
		"type":    record.Type,
		"name":    record.Name,
		"content": record.Content,
		"ttl":     record.TTL,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", zoneID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("cloudflare api auth failed (status %d): token may be revoked or expired", resp.StatusCode)
	}
	return fmt.Errorf("cloudflare api returned status %d", resp.StatusCode)
}

func (c *CloudflareDNS) deleteDNSRecord(ctx context.Context, zoneID string, record DNSRecord) error {
	// List and delete by match. Idempotent: absence is success.
	existing, err := c.listDNSRecords(ctx, zoneID, record.Type, record.Name)
	if err != nil {
		return err
	}
	for _, r := range existing {
		if r.Content != record.Content {
			continue
		}
		url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", zoneID, r.ID)
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		resp, err := c.client.Do(req)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("cloudflare api auth failed (status %d): token may be revoked or expired", resp.StatusCode)
		}
	}
	return nil
}