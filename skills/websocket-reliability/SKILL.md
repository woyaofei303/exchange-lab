---
name: websocket-reliability
description: Build or audit reliable Go WebSocket market-data producers and consumers. Use for exchange-lab changes involving coder/websocket, streaming decoders, heartbeat, reconnect/backoff, sequence validation, duplicate or gap handling, NATS publication, Redis latest-value caches, slow consumers, or disconnect tests.
---

# WebSocket Reliability

Keep transport recovery separate from market-data correctness. Prefer the repository simulator for deterministic work; make third-party read-only feeds optional adapters.

## Workflow

1. Read `references/reliability-checklist.md` before changing a WebSocket producer, consumer, decoder, or reconnect policy.
2. Inspect `internal/marketdata/protocol.go`, `sequencer.go`, and `worker.go` before changing adapters. Keep library types outside the protocol and worker interfaces.
3. Write failing tests for every new state transition before implementation. Cover the smallest affected layer and at least one real WebSocket handshake when adapter behavior changes.
4. Preserve the single-reader invariant. Run Ping concurrently with an active reader; never call `Read` and `Reader` concurrently.
5. Validate before state mutation. Reject unknown JSON fields, oversized messages, numeric JSON decimals, invalid symbols, ambiguous payloads, and unsupported versions.
6. Apply sequence checks per symbol and stream. Require a snapshot for a new stream, drop exact duplicates and old events, and report/reconnect on forward gaps.
7. Bound every wait: read lifecycle, heartbeat, publish/cache operation, message size, reconnect count, exponential delay, and process shutdown.
8. Publish only accepted normalized events. Treat NATS publish failure as a broken session. Treat Redis ticker writes as rebuildable cache failures, never as proof that an event was durably processed.
9. Log fixed error messages and non-sensitive state only. Do not log raw frames, URLs with credentials, downstream error strings, tokens, or user identities.
10. Verify the targeted tests, race detector, real pipeline, and fault recovery before completion.

## Required Verification

```bash
go test ./internal/marketdata ./internal/platform/websocketfeed ./internal/platform/natspublisher
go test -race ./internal/marketdata ./internal/platform/websocketfeed ./internal/platform/natspublisher
make check
API_HOST_PORT=18080 BASE_URL=http://127.0.0.1:18080 make up smoke
make market-check
```

For protocol, reconnect, cache, or Compose changes also run:

```bash
make integration
API_HOST_PORT=18080 BASE_URL=http://127.0.0.1:18080 make acceptance
make market-check
```

Do not claim reliability from a happy-path connection alone. Record which disconnect, malformed-frame, duplicate, out-of-order, gap, heartbeat, slow-downstream, and retry-exhaustion cases were exercised.
