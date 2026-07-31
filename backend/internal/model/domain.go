package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Domain is a customer-owned hostname attached to exactly one tenant site.
type Domain struct {
	bun.BaseModel `bun:"table:domains,alias:d"`

	ID                      uuid.UUID       `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	AccountID               uuid.UUID       `bun:"account_id,notnull,type:uuid" json:"-"`
	SiteID                  uuid.UUID       `bun:"site_id,notnull,type:uuid" json:"site_id"`
	Hostname                string          `bun:"hostname,notnull" json:"hostname"`
	Status                  string          `bun:"status,notnull" json:"status"`
	VerificationTokenDigest []byte          `bun:"verification_token_digest,notnull" json:"-"`
	VerificationExpiresAt   time.Time       `bun:"verification_expires_at,notnull" json:"verification_expires_at"`
	VerificationConsumedAt  *time.Time      `bun:"verification_consumed_at" json:"-"`
	VerifiedAt              *time.Time      `bun:"verified_at" json:"verified_at,omitempty"`
	DNSProvider             string          `bun:"dns_provider,notnull" json:"dns_provider"`
	DNSZoneID               *string         `bun:"dns_zone_id" json:"-"`
	DNSRecordIDs            json.RawMessage `bun:"dns_record_ids,type:jsonb,notnull" json:"-"`
	CertStatus              string          `bun:"cert_status,notnull" json:"cert_status"`
	CertExpiresAt           *time.Time      `bun:"cert_expires_at" json:"cert_expires_at,omitempty"`
	CertObservedAt          *time.Time      `bun:"cert_observed_at" json:"cert_observed_at,omitempty"`
	CertAutoRenew           bool            `bun:"cert_auto_renew,notnull" json:"cert_auto_renew"`
	LastReconciledAt        *time.Time      `bun:"last_reconciled_at" json:"last_reconciled_at,omitempty"`
	IdempotencyKey          *string         `bun:"idempotency_key" json:"-"`
	LastError               *string         `bun:"last_error" json:"last_error,omitempty"`
	CreatedAt               time.Time       `bun:"created_at,notnull,default:now()" json:"created_at"`
	UpdatedAt               time.Time       `bun:"updated_at,notnull,default:now()" json:"updated_at"`
	DeletedAt               *time.Time      `bun:"deleted_at" json:"deleted_at,omitempty"`
}

// DomainPending and related constants describe persisted domain lifecycle states.
const (
	DomainPending      = "pending"
	DomainVerifying    = "verifying"
	DomainDNSPending   = "dns_pending"
	DomainProvisioning = "provisioning"
	DomainActive       = "active"
	DomainFailed       = "failed"
	DomainDeleting     = "deleting"
	DomainDeleted      = "deleted"
)

// CertNone and related constants describe observed certificate lifecycle states.
const (
	CertNone     = "none"
	CertIssuing  = "issuing"
	CertActive   = "active"
	CertExpiring = "expiring"
	CertError    = "error"
)

// DNSProviderManual and related constants identify supported DNS provider modes.
const (
	DNSProviderManual     = "manual"
	DNSProviderCloudflare = "cloudflare"
)

// JobVerifyDomain and related constants identify durable domain worker jobs.
const (
	JobVerifyDomain             = "verify_domain"
	JobProvisionDomain          = "provision_domain"
	JobDeprovisionDomain        = "deprovision_domain"
	JobReconcileDomain          = "reconcile_domain"
	JobObserveDomainCertificate = "observe_domain_certificate"
)

// AuditDomainAttached and related constants identify domain audit events.
const (
	AuditDomainAttached            = "domain.attached"
	AuditDomainChallengeRotated    = "domain.challenge_rotated"
	AuditDomainVerificationQueued  = "domain.verification_queued"
	AuditDomainVerified            = "domain.verified"
	AuditDomainDNSRecordsEnsured   = "domain.dns_records_ensured"
	AuditDomainProvisioned         = "domain.provisioned"
	AuditDomainCertificateObserved = "domain.certificate_observed"
	AuditDomainReconciled          = "domain.reconciled"
	AuditDomainDetachQueued        = "domain.detach_queued"
	AuditDomainDetached            = "domain.detached"
	AuditDomainFailed              = "domain.failed"
)
