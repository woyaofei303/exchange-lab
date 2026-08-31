---
name: ledger-safety
description: Implement or review exchange-lab balance, margin, fee, funding, PnL, liquidation, insurance, faucet, and ledger changes. Use whenever code can create, lock, release, debit, credit, transfer, reconcile, or expose monetary values.
---

# Ledger Safety

Treat money movement as a conservation proof, not a CRUD update.

## Workflow

1. Read `docs/adr/ADR-001-fixed-point-domain-values.md`, `docs/adr/ADR-002-double-entry-ledger.md`, the relevant migration, and `references/checklist.md`.
2. Write a failing test for the invariant or failure boundary before production code.
3. Keep money, margin, fee, rate, and PnL as integer atoms plus an authoritative scale. Use big integers for overflow-prone intermediates; make rounding explicit.
4. Build balanced same-asset entries first. Reject zero, mixed-asset, unbalanced, negative-balance, overflow, and partial-state results.
5. In one database transaction: acquire deterministic locks, detect idempotent replay, append ledger facts, verify conservation, update balance/position snapshots, write outbox/audit facts, then commit.
6. Return the original result for an identical replay and a conflict for changed parameters. Never derive identity or ownership from request fields.
7. Add a reconciliation query and an integration test against real PostgreSQL. Exercise duplicate and concurrent requests.
8. Keep logs and AI context free of passwords, tokens, DSNs, raw provider errors, and unnecessary personal data.

Never let an AI tool directly execute balance, order, leverage, transfer, liquidation, or ledger mutations. AI may explain read-only snapshots and propose code changes that pass these gates.
