-- account_memberships: links better-auth users (auth.user.id) to tenant
-- accounts (public.accounts). One active membership per user for MVP; role is
-- server-owned and never accepted from client input (SECURITY §4, ADR 0006).
CREATE TABLE account_memberships (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id  UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL,
    role        TEXT NOT NULL DEFAULT 'customer'
                CHECK (role IN ('customer', 'admin')),
    status      TEXT NOT NULL DEFAULT 'active'
                CHECK (status IN ('active', 'suspended', 'disabled')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id)
);

CREATE INDEX idx_account_memberships_account_id
    ON account_memberships (account_id);

CREATE INDEX idx_account_memberships_role
    ON account_memberships (role)
    WHERE role = 'admin';
