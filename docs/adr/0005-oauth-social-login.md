# ADR 0005: Social login (Google & GitHub) alongside password auth

- **Status:** Accepted
- **Date:** 2026-07-03
- **Deciders:** Core team

## Context

Phase 1 plans email + password authentication with JWTs issued by the Go
backend. We also want customers to sign in with Google or GitHub — lower
sign-up friction and no password to manage for users who prefer it.

The dashboard already has a BFF (Next.js) and the backend is the system of
record that issues its own access/refresh tokens ([`../SECURITY.md`](../SECURITY.md)).
Delegating session handling to a frontend auth library (NextAuth class) would
split token issuance across two systems; the JWT/refresh model is already
specified backend-side.

## Decision

**OAuth 2.0 / OIDC authorization-code flow, handled by the Go backend.**
Google and GitHub at launch; password login remains available.

- The backend owns the flow: `GET /auth/oauth/{provider}` redirects to the
  provider with a `state` (CSRF) parameter; `GET /auth/oauth/{provider}/callback`
  exchanges the code, fetches the verified profile, then issues the **same JWT
  access + refresh tokens** as password login. One session model, two entry doors.
- Identities live in a new **`user_identities`** table
  (`provider`, `provider_user_id` unique per provider) so one user can have
  password + Google + GitHub linked simultaneously. `users.password_hash`
  becomes **nullable** — OAuth-only users have no password.
- **Auto-linking policy:** a social login whose **provider-verified email**
  matches an existing user links to that user; unverified emails never
  auto-link (account-takeover vector). Otherwise a new account is created.
- Client library: **`golang.org/x/oauth2`** (semi-stdlib, no framework), with
  **PKCE (S256)** on every flow — built into the library
  (`GenerateVerifier`/`S256ChallengeOption`). Provider credentials come from
  config/secret manager, never committed.
- **Provider specifics** (verified against provider docs):
  - *Google* is OIDC — scopes `openid email profile`; identity comes from the
    ID token (verify RS256 via Google's JWKS): `sub` → `provider_user_id`,
    `email_verified` → the auto-link gate.
  - *GitHub* is plain OAuth (no ID token) — scopes `read:user user:email`;
    `GET /user` gives the numeric `id` → `provider_user_id` (the `email` field
    there is often null), then `GET /user/emails` gives the list with
    `verified`/`primary` flags — auto-link only on `primary && verified`.

## Frontend & UX

The flow is backend-owned, so the UI is thin — it triggers a redirect and never
touches a token. Three decisions that aren't obvious from the backend design:

- **Social buttons are a top-level navigation, not a `fetch`.** The
  authorization-code flow needs a full-page browser redirect, so the button is a
  plain `<a href="/api/v1/auth/oauth/{provider}">`, **not** a call through
  `lib/api-client.ts`. This is the sanctioned exception to
  [`FRONTEND.md §3`](../FRONTEND.md#3-data-fetching--the-bff) ("components never
  call fetch directly"): the backend sets the httpOnly cookie in the callback and
  redirects to the dashboard, so the client never sees a token — it *reinforces*
  the BFF model rather than breaking it. Buttons sit above an "or" divider, with
  email/password below; the register page mirrors it.
- **Brand icons are the sanctioned exception to the one-icon-set rule.** Google
  and GitHub logos are not in Lucide (brand glyphs were dropped upstream), so they
  ship as inline brand SVGs. This is the only permitted departure from
  [`UI_GUIDELINES.md §2`](../UI_GUIDELINES.md#2-consistency--design-tokens)
  ("Lucide is the only icon set"); Lucide remains the sole set everywhere else.
  Follow Google's button branding guidance for its control.
- **Failures come back as `/login?error=<code>`**, mapped to plain-language copy
  (state design is mandatory — [`UI_GUIDELINES.md §4`](../UI_GUIDELINES.md#4-state-design-mandatory-per-view)):
  `access_denied` → "Sign-in cancelled."; `invalid_state` → "That took too long —
  please try again."; `email_unverified` → sign in with a password, then link the
  provider in Settings. The last case is the user-facing half of the
  verified-email auto-link gate — without it the user hits a dead-end error.

**Settings → Connected accounts** lists each provider with Connect/Disconnect
(where a password-first user links Google later, or an unverifiable email links
manually). Disconnect is refused when it would remove the account's only remaining
login method (no password **and** one identity left) — an anti-lockout guard,
behind a confirm dialog ([`UI_GUIDELINES.md §7`](../UI_GUIDELINES.md#7-destructive-actions)).

## Consequences

**Easier:** lower sign-up friction; no NextAuth/second session system; refresh
rotation, revocation, RBAC, and audit logging apply unchanged to social logins.

**Harder / accepted cost:** two external identity dependencies (an outage
blocks that provider's logins — password login remains the fallback); a
provider-deleted email or changed `provider_user_id` needs support tooling;
callback URLs must be registered per environment in each provider's console.
