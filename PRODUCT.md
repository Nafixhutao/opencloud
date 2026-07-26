# Product

## Register

product

## Users

OpenCloud serves two authenticated audiences:

- Customers who want to create and operate websites without learning the
  underlying Docker, Caddy, DNS, or database infrastructure.
- Platform operators who manage users, hosting capacity, queue health, and
  cross-account incidents through explicit audited admin paths.

Users are task-focused. They need to understand current state, start a safe
operation, and know what happens next without reading infrastructure jargon.

## Product Purpose

OpenCloud is a custom shared-hosting control plane. It turns account, site,
domain, database, and operational workflows into a clear self-service product
while preserving tenant isolation and automation-first operations.

Success means a customer can provision and manage a working site from the
dashboard, while an operator can diagnose and control the platform without
exposing backend credentials or provider-specific controls.

## Brand Personality

Clear, trustworthy, calm.

The voice is direct and technically honest. It communicates pending work,
failures, limitations, and next steps without alarmism or vague marketing
language.

## Anti-references

- Dense legacy hosting panels that expose every server primitive at once.
- Generic SaaS dashboards made from repetitive icon cards and decorative
  gradients.
- Consumer-cloud interfaces that hide operational state behind vague labels.
- Security theater, fake success states, or claims that external services are
  active before they are configured and tested.
- Novel controls that replace familiar tables, forms, status badges, and
  confirmation patterns without improving the task.

## Design Principles

1. Make state and the next action obvious. Every asynchronous operation shows a
   pending, success, or actionable failure state.
2. Hide provider complexity, not operational truth. Customers see product
   concepts; operators retain the evidence needed to act safely.
3. Protect the tenant boundary in the interface. Customer surfaces never imply
   cross-account reach, while global admin paths are visibly distinct.
4. Use familiar controls consistently. The interface should disappear into the
   hosting task rather than demand relearning.
5. Be honest about irreversible actions and external dependencies. Destructive
   operations name their target, and unavailable capabilities are not presented
   as complete.

## Accessibility & Inclusion

Target WCAG 2.1 AA. Critical workflows must be keyboard operable, screen-reader
understandable, usable without color alone, and respectful of reduced-motion
preferences. Forms require visible labels, field-level errors, and preserved
input after failure. Responsive layouts must remain usable on phone and laptop.
