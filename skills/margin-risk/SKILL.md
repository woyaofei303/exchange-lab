---
name: margin-risk
description: Implement or review exchange-lab leverage, risk tiers, order margin, position margin, isolated margin, cross margin, available balance, or pre-trade risk snapshots.
---

# Margin Risk

1. Start with failing boundary tests for leverage, tier transitions, partial fills, opposing positions, open-order reservation, duplicate requests, and concurrent snapshots.
2. Use exact integer atoms and conservative, documented rounding for notional, IM, MM, PnL, and available balance.
3. Use one formula component for pre-trade and post-trade calculations.
4. Lock balance, margin account, position, and order rows in deterministic order.
5. Compare and increment `risk_version` inside the same serializable transaction.
6. Prove a request cannot reuse already reserved available balance.
7. For isolated mode, preserve the position liquidation boundary; for cross mode, aggregate every USDT position and open order.
8. Add reconciliation and duplicate/concurrency integration tests before approval.

Never allow AI to change leverage or margin. AI may only explain a server-computed snapshot.
