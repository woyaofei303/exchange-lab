# Matching Engine v1

## Deterministic rules

- One book per symbol; v1 supports limit orders.
- Bids sort by highest price first, asks by lowest price first.
- Within one price level, smaller accepted stream sequence has FIFO priority.
- A buy is marketable when its limit is at least the best ask; a sell when its limit is at most the best bid.
- Execution price is the resting maker price. Quantity is `min(taker remaining, maker remaining)` and must be positive.
- Fully filled orders leave the book; partially filled orders retain their original queue priority; an unfilled taker rests at the tail of its price level.
- Cancel removes only the named resting order. Repeating a cancel is not a second state transition.

The pure engine uses integer price/quantity atoms and has no database, NATS, clock, randomness or goroutine. Given the same ordered inputs and starting snapshot it returns byte-for-byte equivalent trades and snapshots.

## Persistence boundary

The `matching-v1` durable consumer processes one message at a time. In one Serializable PostgreSQL transaction it deduplicates the source event, locks the stream offset, rebuilds the pure engine from `matching_book_orders`, applies the event, persists the new book/trades/order states/matching events/trade Outbox, updates the offset and writes a receipt. ACK happens only after commit.

This v1 snapshot approach favors auditability and teaching clarity over maximum throughput. It is safe for the single-symbol lab but rewrites the symbol book per event; a production design would use partitioned single-writer ownership, an append log and periodic snapshots.

## Complexity

Price levels are sorted slices and FIFO queues. Best-price access is O(1); adding a new price level sorts level keys; matching is O(number of fills). The local Apple M1 Pro / darwin arm64 regression baseline for 100 makers plus one taker is recorded by the repeatable benchmark command rather than treated as a universal SLA:

```bash
go test -run '^$' -bench BenchmarkEngineMatch -benchtime=100x ./internal/matching
```

## Known v1 limits

- One-way limit-order matching only; v1 does not accept market orders.
- No self-trade prevention policy yet.
- A single durable consumer serializes all symbols.
- `position-v1` settles executions into positions, margin and fees; external clearing and a production maker/taker fee schedule remain out of scope.
