# ADR 0006: better-auth is the identity provider (Go becomes a resource server)

- **Status:** Accepted — **supersedes [ADR 0005](0005-oauth-social-login.md)**
- **Date:** 2026-07-05
- **Deciders:** Core team

## Context

[ADR 0005](0005-oauth-social-login.md) had the **Go backend** own every auth
path: email/password (argon2id), Google/GitHub OAuth (`golang.org/x/oauth2` +
PKCE), and JWT access/refresh issuance, with identities in a `user_identities`
table. That is a lot of security-sensitive plumbing to build and maintain
ourselves (token rotation, verification emails, provider quirks, 2FA later).

We are adopting **[better-auth](https://better-auth.com)** — a framework-agnostic
TypeScript auth framework — to own identity in the **Next.js BFF** instead. It
ships email/password, social providers, sessions, email verification, and (via
plugins) 2FA/passkey/SSO out of the box, and it can hand a **JWT** to other
services. This reverses 0005's "Go owns auth" decision.

## Decision

**better-auth, mounted in the Next.js BFF, is the identity provider. The Go
backend stops issuing tokens and becomes a resource server that validates
better-auth's JWTs.**

### Ownership split

- **better-auth (Next.js server)** owns email/password, Google + GitHub social
  login, sessions, and email verification. Mounted at
  `app/api/auth/[...all]/route.ts` via `toNextJsHandler(auth)`; config in
  `lib/auth.ts` (`socialProviders`, `emailAndPassword`).
- **Go backend** owns the platform domain (tenancy, sites, DBs, jobs, billing)
  and remains the system of record **for the hosting domain** — but no longer
  for identity. It authorizes requests by **validating better-auth JWTs**, not
  by issuing its own.

### Tokens — JWT + JWKS

- Enable the **`jwt` plugin**; better-auth signs asymmetric JWTs and exposes a
  **JWKS endpoint**. The BFF attaches the JWT to backend calls (still in an
  httpOnly cookie, per [`../SECURITY.md`](../SECURITY.md#3-tokens-in-the-frontend) — tokens never
  reach client JS).
- Go validates each JWT against the cached JWKS (`golang-jwt/jwt/v5` + a JWKS
  fetcher). Go keeps **no** password/OAuth code and issues **no** tokens.
- The session lifetime/rotation model is better-auth's; Go trusts `exp`/`iss`/
  `aud` and the signature. This adds one runtime trust boundary (Go → the BFF's
  JWKS), documented in [`../SECURITY.md`](../SECURITY.md).

### Database

- better-auth owns four core tables — **`user`, `session`, `account`,
  `verification`** (Postgres adapter). `account` holds provider links **and** the
  password hash (better-auth's built-in scrypt by default; configurable). These
  are managed by **better-auth's own migrations/CLI**, not Bun.
- **Naming collision:** better-auth's `account` (a provider credential link) is
  **not** OpenCloud's tenant `accounts` table. Schema separation resolves it —
  better-auth's tables live in a dedicated **`auth` Postgres schema**
  (`auth.user`, `auth.account`, …); the domain stays in `public`
  (`public.accounts` = the tenant, unchanged). Domain rows reference
  `auth.user.id`; **tenant scoping is unchanged** — every customer query is still
  scoped by `account_id`.
- The 0005 additions are dropped: no `user_identities` table, no nullable
  `users.password_hash` (there is no Go-owned `users` table anymore).

### Frontend

- Login/register call the **`better-auth/react`** client
  (`authClient.signIn.email`, `authClient.signIn.social({ provider: "google" })`).
  This **replaces** the 0005 "social button is a top-level redirect, not a fetch"
  detail — better-auth's client owns the redirect. The email/password form stays
  react-hook-form + zod ([`../FRONTEND.md §6`](../FRONTEND.md#6-forms--validation)).
- Google/GitHub brand SVGs are still the sole exception to the Lucide-only rule.

## Consequences

**Easier:** far less security-sensitive Go code (no token issuance, password
hashing, or OAuth exchange to maintain); batteries included (verification, 2FA,
passkey, SSO available as plugins); one well-tested auth surface.

**Harder / accepted cost:**
- Identity now lives in the **TypeScript/BFF layer**, not Go. Go is no longer the
  system of record for identity — a real departure from the original architecture
  ([`../../ARCHITECTURE.md`](../../ARCHITECTURE.md)); Go trusts an externally-issued JWT.
- Two migration systems against one database (better-auth CLI for `auth.*`, Bun
  for `public.*`); the split must stay disciplined.
- A new trust boundary (Go → BFF JWKS) and a new dependency on the BFF being
  up for any auth to work.
- Supersedes ADR 0005 wholesale; `x/oauth2`, argon2id-for-passwords, and the
  `user_identities` design are no longer part of the plan.
