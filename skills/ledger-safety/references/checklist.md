# Ledger Change Checklist

- Exact atoms and scale; no `float32`, `float64`, implicit rounding, or JSON money numbers.
- Every transaction has at least two nonzero, same-asset entries whose checked sum is zero.
- Available and locked balances remain nonnegative and cannot overflow.
- Locks are acquired in deterministic order; transaction isolation and retry semantics are explicit.
- Idempotency covers identity, operation, and parameters; replay returns the original result.
- Ledger facts, snapshots, business state, outbox, and audit commit or roll back together.
- Append-only facts are not updated or deleted by normal business code.
- Reconciliation detects missing sides, nonzero sums, snapshot drift, duplicates, and orphan facts.
- Unit, integration, concurrency, and rollback tests exist before enabling the write path.
- Errors and logs are sanitized; AI and browser clients receive only necessary decimal strings.
