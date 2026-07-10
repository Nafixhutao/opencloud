# ADR 0007: Astryx alongside shadcn/ui, split by route group

- **Status:** Accepted
- **Date:** 2026-07-05
- **Deciders:** Core team

## Context

The dashboard standardized on **shadcn/ui + Tailwind** as the single component
system, with an explicit "no second component library, no CSS-in-JS" rule
([`../CLAUDE.md`](../CLAUDE.md) §6, [`../FRONTEND.md §5`](../FRONTEND.md#5-components--styling)).

We are adopting **[Astryx](https://github.com/facebook/astryx)** — a React +
StyleX design system (customizable foundations, components, templates, themes;
built for both human and AI-agent development) — for the **public marketing
surface**. Astryx is StyleX-based (CSS-in-JS-ish, but compiled to atomic CSS),
and its docs show it **coexisting with Tailwind via CSS layers**, so this is
additive rather than a rip-out.

## Decision

**Two component systems, one per route group. No screen mixes both.**

- **Astryx owns `app/(marketing)`** — landing, pricing, and other public pages.
  Its themes/templates/components give the marketing surface a polished, cohesive
  design system. GSAP stays here (marketing-only) and now layers over Astryx.
- **shadcn/ui owns `app/(dashboard)` and `app/(admin)`** — the control panel.
  Unchanged: the Phase-1 rollout matrix and per-need `add` still apply
  ([`../FRONTEND.md §5`](../FRONTEND.md#5-components--styling)).
- **Boundary rule:** no Astryx primitives in the dashboard/admin; no shadcn
  primitives in marketing. A shared component that needs both worlds is a smell —
  duplicate it per system instead.

### Coexistence

Tailwind stays the shared utility layer for both. Astryx slots in through its
documented CSS-layer setup:

```css
@layer reset, theme, base, astryx-base, astryx-theme, components, utilities;
@import 'tailwindcss/theme.css' layer(theme);
@import 'tailwindcss/preflight.css' layer(base);
@import '@astryxdesign/core/reset.css';
@import '@astryxdesign/core/astryx.css';
@import '@astryxdesign/theme-neutral/theme.css';
@import '@astryxdesign/core/tailwind-theme.css';
@import 'tailwindcss/utilities.css' layer(utilities);
```

Build additions (marketing only): `@stylexjs/stylex`, `@astryxdesign/core`,
`@astryxdesign/theme-neutral`, `@astryxdesign/build` (postcss + babel plugins).
The `(marketing)` layout wraps children in Astryx `Theme` +
`LinkProvider` (bound to `next/link`).

### Usage plan (per surface)

| Surface | System | Uses |
|---|---|---|
| `app/(marketing)` | **Astryx** (+ GSAP, Tailwind utils) | landing, pricing, features, hero, footer — templates + themed components |
| `app/(dashboard)` | **shadcn/ui** + Tailwind | control panel (Phase-1 matrix in `FRONTEND.md §5`) |
| `app/(admin)` | **shadcn/ui** + Tailwind | operator panel |
| auth screens (`login`/`register`) | **shadcn/ui** | forms via better-auth client ([ADR 0006](0006-better-auth-identity-provider.md)) |

Astryx is introduced when the marketing surface is next reworked (not an urgent
rip-and-replace of the working landing) — per-need, matching the shadcn approach.

## Consequences

**Easier:** marketing gets a real design system (templates, theming, agent-
friendly) instead of hand-rolled CSS; the app keeps shadcn's control-panel
ergonomics; the two never fight because they never share a screen.

**Harder / accepted cost:** two component systems and a **StyleX build toolchain**
(babel + postcss) added to the marketing build; contributors must know which
system a route group uses; larger dependency surface. This **relaxes** the
"single component library / no CSS-in-JS" rule ([`../CLAUDE.md`](../CLAUDE.md) §6,
[`../FRONTEND.md §5`](../FRONTEND.md#5-components--styling)) to a **per-route-group
boundary** — the rule still holds *within* a route group.
