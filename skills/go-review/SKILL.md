---
name: go-review
description: Review exchange-lab Go changes for correctness, concurrency, module boundaries, errors, context propagation, exact arithmetic, tests, and operational behavior.
---

# Go Review

1. Identify the domain invariant and owning module before examining style.
2. Trace inputs through validation, exact arithmetic, transaction, event, and response boundaries.
3. Reject float use for price, quantity, money, margin, fee, funding, or PnL.
4. Check context deadlines, sanitized errors, deterministic cleanup, bounded goroutines, queues, retries, and payloads.
5. Check serializable transactions, lock order, idempotency, duplicate events, and replay.
6. Require RED/GREEN evidence, race tests, integration coverage, and failure-path assertions.
7. Prefer a small deep module over forwarding wrappers and duplicated policy.
8. Report concrete defects by severity with file/line evidence and a minimal fix.
