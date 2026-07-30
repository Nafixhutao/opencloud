-- Phase 3 domain management, DNS verification, and certificate status.
-- This is additive: shipped Phase 1 and Phase 2 migrations remain immutable.
CREATE TABLE domains (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id        UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    site_id           UUID REFERENCES sites(id) ON DELETE SET NULL,
    hostname          TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN (
                          'pending', 'verifying', 'verified',
                          'active', 'failed', 'deleting', 'deleted'
                      )),
    verification_type TEXT CHECK (verification_type IN ('txt')),
    verification_token TEXT,
    verified_at       TIMESTAMPTZ,
    dns_provider      TEXT NOT NULL DEFAULT 'manual'
                      CHECK (dns_provider IN ('manual', 'cloudflare')),
    dns_zone_id       TEXT,
    cloudflare_meta   JSONB,
    cert_status       TEXT NOT NULL DEFAULT 'none'
                      CHECK (cert_status IN (
                          'none', 'issuing', 'active',
                          'expiring', 'revoked', 'error'
                      )),
    cert_expires_at   TIMESTAMPTZ,
    cert_auto_renew   BOOLEAN NOT NULL DEFAULT true,
    last_error        TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMPTZ,
    UNIQUE (hostname)
);

CREATE INDEX idx_domains_account_created
    ON domains (account_id, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_domains_site
    ON domains (site_id)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_domains_status
    ON domains (status)
    WHERE status IN ('active', 'verifying');
CREATE INDEX idx_domains_hostname_active
    ON domains (hostname)
    WHERE status = 'active' AND verified_at IS NOT NULL AND deleted_at IS NULL;