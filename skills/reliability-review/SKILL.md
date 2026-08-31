---
name: reliability-review
description: Review exchange-lab concurrency, retries, resource bounds, deterministic replay, timeouts, backpressure, recovery, load behavior, and fault-injection evidence.
---

# Reliability Review

1. State the invariant, workload, machine, duration, and failure being tested.
2. Run unit, integration, race, bounded fuzz smoke, E2E, and migration tests before performance claims.
3. Reject unbounded goroutines, channels, queues, message sizes, retries, history queries, caches, and cardinality.
4. Require explicit context deadlines, finite backoff, retry budgets, parking paths, and shutdown joins.
5. Prove duplicate and out-of-order financial events preserve money and state invariants.
6. Inject PostgreSQL, Redis, NATS, AI, and price-source failures; verify degraded behavior and recovery.
7. Save raw results with machine and dataset metadata under `output-tdd/performance/`.
8. Verify backup restore, replay, reconciliation, and bad-version rollback before release.

Do not turn a local benchmark into a production SLO. Do not hide failed or noisy runs.
