-- accounts: the tenant boundary (DATABASE.md §3). Every customer-owned row
-- carries account_id and references this table. Identity (auth.*) is owned by
-- better-auth, not Bun (ADR 0006) — this migration touches only public.*.
CREATE TABLE accounts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active'
                CHECK (status IN ('active','suspended','closed')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
