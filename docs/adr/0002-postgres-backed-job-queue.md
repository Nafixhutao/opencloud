# ADR 0002: Postgres-backed job queue instead of a Redis queue

- **Status:** Accepted
- **Date:** 2026-07-02
- **Deciders:** Core team

## Context

Provisioning work is asynchronous: a request creates a resource row
(`status=provisioning`) and enqueues a job the worker executes against Docker.
The original design queued jobs in Redis with a `jobs` table in PostgreSQL
"mirroring" queue state for audit and recovery.

That design has a dual-write flaw: the Redis enqueue is not part of the
PostgreSQL transaction. If the transaction commits but the enqueue fails, the
resource is stuck in `provisioning` forever; if the enqueue succeeds but the
transaction rolls back, a ghost job processes a resource that doesn't exist. The
mirror table also created a third copy of queue truth to keep consistent.

Expected job volume is small (provisioning actions per day, not per second) — a
hosting control plane is not a high-throughput task system.

## Decision

The **`jobs` table in PostgreSQL is the queue**. Enqueue is an `INSERT` inside
the caller's transaction, so the resource row and its job commit or roll back
together. Workers claim jobs with `FOR UPDATE SKIP LOCKED` (safe with multiple
workers), retry with backoff via `run_at`, and a periodic reaper re-queues jobs
stuck in `running` past a timeout.

Redis remains for cache, sessions, and rate limiting only.

## Consequences

**Easier:** no dual-write or mirror-sync problem; exactly one source of queue
truth; enqueue is transactional; audit and recovery come free from the table;
one less moving part in dev, CI, and production.

**Harder / accepted cost:** workers poll (small constant DB load) instead of
blocking on a Redis pop; queue throughput is bounded by PostgreSQL. At this
platform's job volume that bound is irrelevant. If volume ever demands a
dedicated queue, supersede this ADR rather than editing it.
