# Frontend — Next.js Dashboard

The OpenCloud dashboard is a Next.js (App Router) application: the customer
self-service UI and the operator admin panel. Contract: [`../CLAUDE.md`](../CLAUDE.md).
Design and UX rules: [`UI_GUIDELINES.md`](UI_GUIDELINES.md).

**Stack:** Next.js (App Router) · React 19 · TypeScript (strict) · Tailwind CSS ·
shadcn/ui (dashboard/admin) · Astryx + StyleX (marketing — [ADR 0007](adr/0007-astryx-alongside-shadcn.md)) ·
Lucide React · GSAP (`@gsap/react`) · Geist fonts via `@fontsource`.
**Auth:** better-auth in the BFF ([ADR 0006](adr/0006-better-auth-identity-provider.md)).

**Approved for the dashboard phase** (add when the need lands, not before):
**TanStack Query** (server state + job-status polling) · **react-hook-form + zod**
(forms — wired through shadcn/ui `Field` primitives) · **TanStack Table** (data tables) ·
**Recharts** (usage charts — what shadcn/ui charts are built on) ·
**Vitest + Testing Library** (tests — [`TESTING.md`](TESTING.md#6-frontend-tests)).
Anything else follows the `CLAUDE.md` §5.4 approval rule.

> **Migration note:** the app has migrated off Vite to a minimal Next.js App
> Router shell at the repo root. Add `components/` and `lib/` when their first
> consumers land; the removed Vite and `src/` scaffolds stay removed.

---

## 1. Folder layout

```
. (repo root)
├── app/
│   ├── (marketing)/        # public landing pages
│   ├── (auth)/             # login/register (shadcn/ui — ADR 0007)
│   ├── (dashboard)/        # authenticated customer area
│   │   ├── layout.tsx       # shell: nav, auth guard
│   │   ├── sites/           # route = folder; page.tsx, loading.tsx, error.tsx
│   │   └── ...
│   ├── (admin)/            # operator area (role-gated)
│   ├── api/                # route handlers — BFF only, no business logic
│   ├── layout.tsx          # root layout, fonts, providers
│   └── page.tsx
├── components/
│   ├── ui/                 # shadcn/ui primitives (generated)
│   └── ...                 # feature components
├── lib/
│   ├── api-client.ts       # typed fetch wrapper → backend
│   ├── auth.ts             # better-auth server configuration
│   ├── auth-client.ts      # browser auth client
│   ├── session.ts          # request-scoped server session helper
│   ├── utils.ts            # cn() and friends
│   └── hooks/              # client hooks
├── public/
├── next.config.ts
└── package.json
```

Route groups `(marketing)`, `(auth)`, `(dashboard)`, `(admin)` separate the
audiences without affecting URLs; `(auth)` holds the login/register screens
(shadcn/ui per [ADR 0007](adr/0007-astryx-alongside-shadcn.md)). Both routes
share one responsive authentication shell; Google/GitHub actions render only
when the corresponding Better Auth credentials are complete.

## 2. Rendering model

- **Server Components by default.** Fetch on the server, ship less JS.
- Add `"use client"` **only** when you need interactivity (state, effects, event
  handlers). Keep client components small and at the leaves.
- Use `loading.tsx` for streaming/suspense and `error.tsx` for error boundaries on
  every async route segment. These are mandatory, not optional.

```tsx
// app/(dashboard)/sites/page.tsx — Server Component
import { api } from "@/lib/api-client";
import { SiteList } from "@/components/site-list";

export default async function SitesPage() {
  const { data } = await api.sites.list();   // runs on the server
  return <SiteList sites={data} />;
}
```

## 3. Data fetching & the BFF

- **All** backend calls go through the typed client in `lib/api-client.ts`.
  Components never call `fetch` directly.
- The Next.js server (route handlers + server components) is the **BFF**: it holds
  the JWT in an httpOnly cookie and attaches it to backend calls. Tokens never
  reach client JavaScript. See [`SECURITY.md`](SECURITY.md#3-tokens-in-the-frontend). (Auth is the
  exception: sign-in/session go through the **better-auth** client, not
  `lib/api-client.ts` — better-auth is mounted at `app/api/auth/[...all]/route.ts`
  and owns the flow — [ADR 0006](adr/0006-better-auth-identity-provider.md).)
- Mutations from client components call a route handler under `app/api/…`, which
  forwards to the backend with the session token. Route handlers contain **no
  business logic** — they are a thin secure proxy.

```ts
// lib/api-client.ts
export class ApiError extends Error {
  constructor(public code: string, message: string, public status: number) { super(message); }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${process.env.API_URL}${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...authHeader(), ...init?.headers },
  });
  const body = await res.json();
  if (!res.ok) throw new ApiError(body.error.code, body.error.message, res.status);
  return body.data as T;
}
```

## 4. State management

- **Server state** (data from the backend) is fetched in Server Components or
  cached route handlers — not mirrored into a global client store. Where the
  client must own it (mutations, job-status polling until `active|failed`),
  use **TanStack Query**; don't hand-roll `useEffect` fetch loops.
- **Client state** is local (`useState`/`useReducer`) or, for cross-tree concerns
  (theme, current user), a small React Context. No Redux unless a real need appears.
- Prefer URL/searchParams for shareable state (filters, pagination) over client state.

## 5. Components & styling

- **shadcn/ui** is the component baseline **for `app/(dashboard)` and
  `app/(admin)`**. Compose its primitives in `components/ui/`. The **marketing**
  surface uses **Astryx** instead ([ADR 0007](adr/0007-astryx-alongside-shadcn.md));
  never mix the two within one route group.
- Generated primitives may be edited, but keep the shadcn structure so upstream
  updates stay mergeable.
- **Dashboard/admin:** Tailwind only, with no CSS-in-JS or ad-hoc stylesheets
  beyond `globals.css`. Marketing may use Astryx's compiled StyleX setup per
  ADR 0007. Compose shared Tailwind class names with `cn()`.
- **Lucide React** for icons — one set, imported per-icon (tree-shaken).
- **GSAP** (`@gsap/react`, `ScrollTrigger`) is for the marketing/landing surface
  only. Dashboard interactions use CSS/Tailwind transitions; don't add a second
  animation library.

```tsx
import { cn } from "@/lib/utils";
import { Server } from "lucide-react";

export function NodeBadge({ online, className }: { online: boolean; className?: string }) {
  return (
    <span className={cn("inline-flex items-center gap-1 text-sm", online ? "text-emerald-600" : "text-red-600", className)}>
      <Server className="size-4" /> {online ? "Online" : "Offline"}
    </span>
  );
}
```

### Component rollout (per-need, not up front)

shadcn/ui is initialized in **Phase 1** ([`../ROADMAP.md`](../ROADMAP.md)) with the
first authenticated screen; primitives are added as each need lands
(`npx shadcn@latest add …`) — never `--all`:

| Phase | `add` | For |
|---|---|---|
| **1 Auth** | `button input label card field dialog sonner dropdown-menu avatar badge skeleton` | login/register/profile, app shell, toasts |
| **2 Provisioning** | `table alert-dialog select tooltip progress sheet` | site/DB lists, async status, destructive delete |
| **3 Domains/DNS/SSL** | `alert-dialog badge button card empty field input separator skeleton spinner table` | domain lifecycle, manual DNS, certificate state, typed detach |
| **4 Email/FTP/cron** | `checkbox tabs` | account toggles |
| **5 Billing** | `chart` | usage charts, invoices |

`Field` primitives wire react-hook-form + zod via `@hookform/resolvers/zod` — one
pattern for every form (§6); the backend always re-validates. GSAP stays
landing-only — dashboard motion is Tailwind + `tw-animate-css` (§5), not a second
lib. The table above is the **dashboard/admin** (shadcn) surface;
`app/(marketing)` uses **Astryx** components/templates instead
([ADR 0007](adr/0007-astryx-alongside-shadcn.md)).

## 6. Forms & validation

- Forms use **react-hook-form + zod** through the shadcn/ui `Field` primitives —
  one pattern for every form; the zod schema is the single client-side
  definition of a form's shape.
- Client-side validation is UX only — the backend **always** re-validates.
  Never trust the browser.
- Show field-level errors mapped from the backend's validation envelope.
- Destructive actions (delete site, suspend account) require an explicit confirm dialog.

## 7. TypeScript rules

- `strict: true`. **No `any`** — use `unknown` + narrowing or a real type.
- Shared shapes (API responses, domain models) live in typed modules and are
  imported, not redeclared inline.
- Keep backend DTOs and frontend types in sync; treat the API envelope as the contract.

## 8. Accessibility & UX

Non-negotiable basics — see [`UI_GUIDELINES.md`](UI_GUIDELINES.md) for the full set:

- Semantic HTML, labeled inputs, visible focus rings, AA contrast.
- Every async view designs its empty / loading / error / success states.
- Keyboard-navigable and responsive (laptop + phone).

## 9. Tooling & scripts

```bash
npm run dev     # next dev — http://localhost:3000
npm run build   # production build
npm run start   # serve production build
npm run lint    # oxlint
npm run test:ui # Vitest + Testing Library dashboard behavior tests
```

- Lint with **oxlint** (configured in `.oxlintrc.json`); type-check with `tsc`.
- No secrets in client code or `NEXT_PUBLIC_*` vars. Anything sensitive stays
  server-side. See [`SECURITY.md`](SECURITY.md).

## Phase 1 UI

- `/account` profile + change password
- `/forgot-password`, `/reset-password`
- `/admin/users` (role-gated)
- BFF routes: `/api/account/profile`, `/api/admin/users/[id]` attach JWT via `lib/api.ts`

## Phase 2 site UI

- `/sites` provides one accessible create form, responsive list/table views,
  explicit transitional statuses, and typed-domain confirmation before delete.
- TanStack Query owns server state and polls every two seconds only while a site
  is transitional. Lifecycle mutations invalidate the tenant's site query.
- Browser calls terminate at thin `/api/sites/*` BFF handlers. Those handlers
  attach the server-side JWT, preserve `Idempotency-Key`, and return generic
  authentication/provider errors without exposing tokens or backend internals.
- `/databases` follows the same server-state boundary for tenant-scoped
  PostgreSQL/MariaDB create/list/delete. It polls only transitional rows and
  requires explicit confirmation before consuming the credential exactly once.
- Database list pagination is carried through the typed browser client and BFF;
  page and size are part of the TanStack Query key, controls are keyboard
  accessible, and deleting the final row on the last page returns to the last
  valid page.
- `/dashboard` reads one tenant-scoped `/api/v1/overview` aggregate instead of
  treating the first paginated site/database arrays as complete collections.
  Total and active metrics therefore remain accurate beyond 25 resources
  without fetching every page into the server component.
- The one-time credential panel is never persisted in browser storage and can be
  hidden immediately. Reloading does not reproduce a consumed password; the
  customer must delete and recreate the database if it is lost.
- The launch site template is intentionally limited to `static`; DNS automation,
  uploads/builds, database backups, and production rollout remain later work.

## Phase 3 domain UI

- `/sites/[id]` presents one restrained domain workspace instead of repeating
  card grids: attach control, paginated status table/list with accessible
  URL-backed previous/next controls, lazy per-row DNS instructions (TXT first,
  A only after proof), certificate evidence, and
  next actions share one visual hierarchy.
- Thin browser-facing BFF routes under `/api/sites/[id]/domains` and
  `/api/domains/[id]/*` validate identifiers/payloads, attach the server-side JWT,
  preserve idempotency, and proxy only the documented Go API contract.
- The attach form uses `FieldGroup`/`Field`, associated labels/descriptions, and
  `aria-invalid`. Mutations disable their control and show a spinner; initial
  loading uses skeletons; API errors remain visible with an explicit retry or
  corrective next action.
- Polling runs only for transitional domain states (`verifying`, `dns_pending`,
  `provisioning`, `deleting`) or certificate `issuing`; terminal state and
  unmount cancel it, `401` redirects to login with a validated same-origin
  return path, and `429` honors `Retry-After` without the global retry.
  Only the visible page polls. Status copy distinguishes
  requested work from observed DNS and TLS evidence, so the UI never invents
  success.
- Copy controls announce success through `aria-live` and change their accessible
  label. Detach uses `AlertDialog`, requires the exact hostname, preserves focus,
  and cannot be submitted until it matches.
- Customer types intentionally omit `account_id`, verification digests,
  provider zone/record identifiers, and credentials. The raw ownership token is
  displayed only in the authenticated manual instruction response.
