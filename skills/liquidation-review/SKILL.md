---
name: liquidation-review
description: Review exchange-lab liquidation, bankruptcy price, insurance fund, ADL, reduce-only liquidation orders, and related ledger settlement changes.
---

# Liquidation Review

1. Confirm the trigger uses a fresh mark price and a versioned server risk snapshot.
2. Verify the job idempotency key includes user, symbol, and risk version.
3. Exercise price gaps, partial fills, duplicate jobs, worker restart, retry exhaustion, and parking.
4. Ensure every liquidation order is isolated from user orders and is reduce-only.
5. Reconcile position, margin, balance, ledger, insurance, and ADL facts in one transaction or explicit saga.
6. Require one bankruptcy-loss path: released collateral, then insurance, then documented teaching-grade ADL.
7. Rank ADL with persisted evidence and stable tie-breaking; cap impact.
8. Preserve immutable trigger/execution/settlement evidence and a usable Runbook.

AI can inspect and explain evidence. It cannot decide, trigger, execute, retry, or cancel liquidation.
