# Exchange Frontend Contract Checklist

## Transport

- REST success and error envelopes are decoded, not type-cast.
- Fetch throws on non-2xx responses and accepts an `AbortSignal`.
- WebSocket frames have a supported `version` and `type`.
- Time is RFC 3339 over the wire and milliseconds only at browser/chart APIs.

## Exact values

- Price, quantity, money, margin, PnL, rates, and ratios remain decimal strings.
- Chart-only `Number(...)` conversion is isolated and labelled approximate.
- Order/risk inputs return to the server as the original exact text or a validated fixed-point representation.

## Lifecycle

- Query keys contain symbol, interval, user, and account as applicable.
- Old requests are aborted and old sockets are closed on identity changes.
- Reconnect uses a bounded backoff and reports state to the user.
- Duplicate candle timestamps replace; older realtime candles do not append after the current bucket.

## UX and tests

- Loading, empty, error, stale, reconnecting, and offline states are distinguishable.
- Unit tests cover malformed data, duplicates, symbol/period change, reconnect, and cleanup.
- Playwright covers first load, a selection change, disconnect, and recovery against running services.
