# Reliability Checklist

## Protocol boundary

- Keep protocol version, stream ID, symbol, monotonic sequence and UTC event time in every envelope.
- Encode financial decimals as JSON strings and validate at most 18 fractional digits; never decode into `float64`.
- Allow exactly one payload matching the declared event type.
- Cap frames before JSON decoding and reject trailing JSON or unknown fields.
- Keep NATS subjects derived from validated symbols and event types, never from unchecked input.

## Connection ownership

- Own exactly one reader goroutine or loop per connection.
- Run heartbeat Ping concurrently with the reader so Pong control frames are consumed.
- Cancel the session on heartbeat failure; wait for the heartbeat goroutine during teardown.
- Close the old connection before sleeping and reconnecting.
- Use context deadlines for dial, Ping, downstream publish/cache writes and shutdown.

## Sequence state

- A new `stream_id` starts only with `orderbook_snapshot`; the snapshot sequence becomes the baseline.
- `received == last`: duplicate; do not publish again.
- `received < last`: out of order; do not roll state backward.
- `received == last + 1`: accept and advance.
- `received > last + 1`: report expected/received without raw payload, close the session and request a fresh snapshot.
- Never advance `last` for rejected, duplicate, out-of-order or gap events.

## Reconnect and backpressure

- Use exponential backoff capped by a configured maximum.
- Bound reconnect attempts (exchange-lab accepts 1–20); return `ErrReconnectExhausted` to the supervisor.
- Keep channels absent or explicitly bounded; do not spawn a goroutine per frame.
- Give slow NATS/Redis operations a deadline. NATS failure breaks the session; Redis latest-ticker failure degrades because the value is rebuildable.
- Use a bounded container restart policy so process supervision cannot silently become an infinite retry loop.

## Test matrix

- Valid snapshot followed by contiguous ticker, trade and delta.
- Invalid JSON, unknown field, oversized payload, binary frame and numeric decimal.
- Duplicate, out-of-order, forward gap and new stream without snapshot.
- Peer close, dial failure, heartbeat failure and context cancellation.
- Slow publisher timeout and cache outage.
- Exact retry count, exponential delays and cap.
- Race test with real `coder/websocket` handshake.
- Runtime NATS subscription plus Redis latest-ticker check after dependency recovery.
