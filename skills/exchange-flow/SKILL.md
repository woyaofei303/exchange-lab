---
name: exchange-flow
description: Implement or verify exchange-lab's complete perpetual-contract flow across UI, API, order, margin, matching, position, funding, liquidation, insurance, ledger, WebSocket, and recovery.
---

# Exchange Flow

1. Use two authenticated funded users and one deterministic BTCUSDT-PERP price scenario.
2. Set leverage and margin mode, place crossing long/short orders, and verify one trade.
3. Trace order events through matching, position settlement, fees, margin locks, ledger entries, and user updates.
4. Verify mark/index/last are distinct concepts and every risk value comes from backend truth.
5. Settle funding twice with the same key/time and prove system conservation and idempotency.
6. Move mark price through a risk boundary and verify one auditable reduce-only liquidation path.
7. Check insurance/ADL evidence and ensure conditional TP/SL cannot expand a position.
8. Repeat after browser refresh, WebSocket reconnect, worker restart, PostgreSQL restore, and event redelivery.
9. Keep AI and MCP tools read-only and user-isolated throughout the flow.

Capture command, machine, dataset, timestamps, and reconciliation results for every final claim.
