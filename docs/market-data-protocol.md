# Market Data Protocol and Failure States

## Scope

The default source is a deterministic local WebSocket simulator. The protocol is replaceable: any read-only public exchange adapter must normalize into the same envelope before sequence checks, NATS publication or Redis caching.

No real credentials, private exchange channel or order write is permitted.

## Transport

- Simulator health: `GET http://127.0.0.1:8090/healthz`
- WebSocket feed: `ws://127.0.0.1:8090/ws`
- Frame type: text only; binary frames are rejected.
- Compression: disabled to keep resource use predictable in the learning environment.
- Maximum message size: default `1 MiB`, configuration limit `16 MiB`.
- Financial decimals: JSON strings with at most 18 fractional digits; JSON numbers are rejected.

## Envelope

Every event contains:

```json
{
  "version": 1,
  "stream_id": "sim-0123456789abcdef",
  "type": "ticker",
  "symbol": "BTCUSDT-PERP",
  "sequence": 2,
  "emitted_at": "2026-09-14T00:00:00Z",
  "ticker": {
    "last_price": "50000.10",
    "mark_price": "50000.10",
    "index_price": "50000.00",
    "price_change": "0.10",
    "volume": "1.000"
  }
}
```

Exactly one payload is allowed and it must match `type`:

- `ticker`: latest last/mark/index price, change and volume.
- `trade`: `trade_id`, `buy|sell`, price and positive quantity.
- `orderbook_snapshot`: non-empty bids and asks; establishes a new stream baseline.
- `orderbook_delta`: one or both sides; zero quantity may represent deletion.

Unknown fields, trailing JSON, invalid symbols, unsupported versions, zero sequences, invalid timestamps and ambiguous payloads are rejected before sequence state changes.

## Sequence State Machine

State is tracked per symbol:

```text
new stream + snapshot       -> accept; set baseline
new stream + other event    -> gap; reconnect for snapshot
sequence == last            -> duplicate; drop
sequence < last             -> out_of_order; drop
sequence == last + 1        -> accept; advance
sequence > last + 1         -> gap; report expected/received; reconnect
```

Rejected, duplicate, out-of-order and gap events never advance state or reach NATS/Redis.

## Delivery

Accepted events are encoded canonically and published through NATS Core:

```text
market.BTCUSDT-PERP.ticker
market.BTCUSDT-PERP.trade
market.BTCUSDT-PERP.orderbook
```

NATS Core is intentionally an ephemeral fan-out bus; it is not the durable source of truth. A publish/flush timeout breaks the current source session so the worker does not silently skip data.

Ticker events update both Redis keys:

```text
exchange-lab:v1:cache:tickers:all
exchange-lab:v1:cache:tickers:symbol:BTCUSDT-PERP
```

Redis failure does not reject an already published market event because ticker cache is rebuildable.

## Heartbeat, Backoff and Shutdown

- One reader owns each connection.
- Ping runs concurrently with the reader; otherwise Pong cannot be consumed reliably.
- Dial, Ping, NATS publish/flush and Redis writes use context deadlines.
- Reconnect delay doubles from `250ms` to a `5s` cap.
- Reconnect attempts are configured from 1 to 20; default is 5, after which the process returns `ErrReconnectExhausted`.
- Compose supervises the process with bounded `on-failure:3`, not an unlimited restart policy.
- Context cancellation stops heartbeat/read/backoff and closes the connection.

## Reproducible Failure Cases

- Disconnect/dial failure: scripted connector and real WebSocket close tests.
- Invalid JSON/unknown fields/numeric decimals: strict decoder and fuzz test.
- Duplicate/out-of-order: state machine drops without republishing.
- Gap/new stream without snapshot: gap report and bounded reconnect.
- Binary/oversized frames: WebSocket adapter rejects before decoding.
- Slow downstream: publisher context deadline ends the session.
- Redis outage: NATS continues; cache resumes after Redis recovery.
- NATS outage: bounded source reconnection; runtime acceptance verifies recovery.
