package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Domain is a customer-owned hostname attached to a site after ownership
// verification. One hostname may belong to exactly one tenant.
type Domain struct {
	bun.BaseModel `bun:"table:domains,alias:d"`

	ID                uuid.UUID       `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	AccountID         uuid.UUID       `bun:"account_id,notnull,type:uuid" json:"account_id"`
	SiteID            *uuid.UUID      `bun:"site_id,type:uuid" json:"site_id,omitempty"`
	Hostname          string          `bun:"hostname,notnull" json:"hostname"`
	Status            string          `bun:"status,notnull" json:"status"`
	VerificationType  *string         `bun:"verification_type" json:"verification_type,omitempty"`
	VerificationToken *string         `bun:"verification_token" json:"-"`
	VerifiedAt        *time.Time      `bun:"verified_at" json:"verified_at,omitempty"`
	DNSProvider       string          `bun:"dns_provider,notnull" json:"dns_provider"`
	DNSZoneID         *string         `bun:"dns_zone_id" json:"dns_zone_id,omitempty"`
	CloudflareMeta    *string         `bun:"cloudflare_meta,type:jsonb" json:"-"`
	CertStatus        string          `bun:"cert_status,notnull" json:"cert_status"`
	CertExpiresAt     *time.Time      `bun:"cert_expires_at" json:"cert_expires_at,omitempty"`
	CertAutoRenew     bool            `bun:"cert_auto_renew,notnull" json:"cert_auto_renew"`
	LastError         *string         `bun:"last_error" json:"last_error,omitempty"`
	CreatedAt         time.Time       `bun:"created_at,notnull,default:now()" json:"created_at"`
	UpdatedAt         time.Time       `bun:"updated_at,notnull,default:now()" json:"updated_at"`
	DeletedAt         *time.Time      `bun:"deleted_at" json:"deleted_at,omitempty"`
}

// Domain lifecycle statuses.
const (
	DomainPending   = "pending"
	DomainVerifying = "verifying"
	DomainVerified  = "verified"
	DomainActive    = "active"
	DomainFailed    = "failed"
	DomainDeleting  = "deleting"
	DomainDeleted   = "deleted"
)

// Certificate lifecycle statuses.
const (
	CertNone     = "none"
	CertIssuing  = "issuing"
	CertActive   = "active"
	CertExpiring = "expiring"
	CertRevoked  = "revoked"
	CertError    = "error"
)

// Verification types.
const (
	VerificationTXT = "txt"
)

// DNS provider backends.
const (
	DNSProviderManual     = "manual"
	DNSProviderCloudflare = "cloudflare"
)

// Domain-related job kinds.
const (
	JobVerifyDomain   = "verify_domain"
	JobProvisionDNS   = "provision_dns"
	JobDeprovisionDNS = "deprovision_dns"
	JobRenewCert      = "renew_cert"
)

// Domain-related audit actions.
const (
	AuditDomainAttachQueued = "domain.attach_queued"
	AuditDomainAttached     = "domain.attached"
	AuditDomainVerified     = "domain.verified"
	AuditDomainProvisioned  = "domain.provisioned"
	AuditDomainFailed       = "domain.failed"
	AuditDomainDetachQueued = "domain.detach_queued"
	AuditDomainDetached     = "domain.detached"
)