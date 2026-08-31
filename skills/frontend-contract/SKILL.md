---
name: frontend-contract
description: Implement or review exchange-lab React integrations with Go REST and WebSocket contracts. Use for API adapters, realtime subscriptions, symbol or interval changes, exact price/quantity display, loading/error/offline states, KLineChart data, and frontend contract tests.
---

# Frontend Contract

Keep transport validation, exact domain values, UI state, and realtime lifecycle explicit.

## Workflow

1. Read `openapi.yaml`, the Go response type, and `references/checklist.md` before changing an adapter.
2. Add a failing decoder, lifecycle, or UI contract test.
3. Decode every REST response and WebSocket frame at the boundary. Reject unsupported versions, unsafe integers, numeric money fields, malformed symbols, and invalid timestamps.
4. Preserve price, quantity, margin, PnL, and money as decimal strings. Convert to JavaScript `number` only inside a visualization adapter that requires it; never reuse that value for orders or risk.
5. Key queries and subscriptions by every input that changes identity. On symbol, interval, user, or account change, abort stale REST requests and close the old realtime subscription before showing new data.
6. Model loading, empty, error, connecting, live, reconnecting, stale, and offline states. Do not represent an unavailable value as zero.
7. Bound reconnect delays, queues, response sizes, and time ranges. Ignore duplicates by stable event identity or bucket timestamp.
8. Run unit tests, production build, and the Playwright reconnect smoke.

## Required Evidence

- Contract test rejects at least one malformed payload.
- Reconnect test proves the old socket cannot update new state.
- Historical and realtime candles share the same timestamp key and do not duplicate the boundary bar.
- Visible connection/freshness state exists.
- No raw server error, token, credential, or untrusted HTML reaches the page.
- Documentation records protocol version, exact decimal rule, time zone, and rollback path.

Do not add an AI write path for order, cancellation, leverage, margin, transfer, or balance changes.
