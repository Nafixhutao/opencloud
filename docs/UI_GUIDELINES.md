# UI / UX Guidelines

Design and interaction rules for the OpenCloud dashboard and admin panel. The
implementation stack and component conventions are in [`FRONTEND.md`](FRONTEND.md);
this is about how the product should look and behave.

**Stack:** Tailwind CSS · shadcn/ui · Lucide React.

---

## 1. Design principles

1. **Clarity over decoration.** This is a control panel — users come to do a task,
   not admire the UI. Reduce the clicks to the goal.
2. **Consistency.** Same primitives, spacing, and patterns everywhere; a user who
   learns one screen can use the next.
3. **Every state is designed** — empty, loading, success, error. No blank screens,
   no dead-end errors.
4. **Feedback is immediate.** The user always knows what happened and what's next.
5. **Hard to break, easy to undo.** Dangerous actions are deliberate and reversible
   where possible.
6. **Respect the user's time.** Sane defaults, remembered preferences, no needless
   confirmations on safe actions.

## 2. Consistency & design tokens

- Use **shadcn/ui** primitives as the baseline; compose, don't reinvent. No second
  component library.
- One spacing scale, one type scale, one color palette — defined in the Tailwind
  config and used via tokens, not ad-hoc values.
- **Lucide React** is the only icon set; consistent sizes (`size-4`/`size-5`) and
  meanings (a trash icon always means delete). Sole exception: Google/GitHub brand
  logos ship as inline SVGs ([ADR 0005](adr/0005-oauth-social-login.md#frontend--ux)).
- Light/dark themes share tokens; never hardcode colors that break a theme.

## 3. Layout & navigation

- Clear separation of the three audiences via route groups: marketing, customer
  dashboard, admin ([`FRONTEND.md`](FRONTEND.md#1-folder-layout)).
- Persistent primary navigation; the user always knows where they are
  (breadcrumbs/active state).
- Content has breathing room; group related actions; primary action is visually
  primary, destructive actions are visually distinct.

## 4. State design (mandatory per view)

| State | Requirement |
|---|---|
| **Loading** | Skeletons over spinners where it reads better; stream with `loading.tsx`. Never a blank screen. |
| **Empty** | Explain what goes here and offer the primary action (e.g. "No sites yet — Create your first site"). |
| **Error** | Plain-language message + a next step (retry, contact support). Use `error.tsx` boundaries. Never show raw errors. |
| **Success** | Confirm via toast/inline state; reflect the new state immediately. |

## 5. Feedback & async operations

- Provisioning is async ([`HOSTING.md`](HOSTING.md)) — show a clear **pending**
  status (e.g. "Provisioning…") and update to active/failed when it resolves.
- Use **optimistic UI** only where safe to roll back; otherwise show pending state.
- Toasts confirm actions; they're brief and don't block. Persistent problems get
  inline messaging, not just a toast.

## 6. Forms

- Controlled inputs, validated client-side for UX and **always** server-side for
  trust ([`SECURITY.md`](SECURITY.md)).
- **Inline, field-level errors** mapped from the API validation envelope
  ([`API.md`](API.md#4-response-envelopes)); summarize at the top for long forms.
- Sensible defaults and helper text; disable submit while pending and show progress.
- Preserve user input on error — never make them retype.

## 7. Destructive actions

- Require an explicit **confirm dialog** for suspend/delete; name the resource in
  the prompt ("Delete **example.com**? This removes all files and databases.").
- For high-impact deletes, require typing the resource name to confirm.
- Offer undo or a grace period where the backend allows it.

## 8. Accessibility (WCAG 2.1 AA — not optional)

- **Semantic HTML** first; `aria-*` only where semantics fall short.
- All inputs have associated **labels**; controls are reachable and operable by
  keyboard with a visible **focus ring**.
- **Contrast** meets AA for text and meaningful UI.
- Icon-only buttons have accessible names; don't convey meaning by color alone.
- Respect `prefers-reduced-motion`; animations are subtle and purposeful.
- Test with keyboard-only navigation and a screen reader for critical flows.

## 9. Responsive design

- Works on a laptop and a phone. Mobile-first Tailwind breakpoints.
- Tables become readable cards/stacks on small screens; primary actions stay reachable.
- No horizontal scroll for primary content; touch targets are adequately sized.

## 10. Performance (UX-facing)

- Server Components + streaming for fast first paint ([`FRONTEND.md`](FRONTEND.md#2-rendering-model)).
- `next/image` for images; code-split heavy client bundles; prefetch on navigation.
- Perceived speed matters: skeletons and optimistic updates keep the UI responsive.

## 11. Content & tone

- Plain, direct language. Say "Delete site", not "Initiate deletion procedure".
- Error messages are honest and actionable — what happened and what to do.
- Consistent terminology with the domain (site, domain, database, node) across UI,
  API, and docs.

## 12. Admin panel specifics

- Denser layouts are acceptable for operators (more data per screen).
- Dangerous operational actions (drain node, suspend account) are clearly gated and
  audited ([`SECURITY.md`](SECURITY.md#12-audit-logging)).
- Surface system health (node status, queue depth) where operators act on it.
