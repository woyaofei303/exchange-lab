# Candle Aggregation and Data Quality

## Event-time policy

- Bucket boundaries always use UTC event time, never worker processing time.
- 1m/5m/1h intervals are half-open ranges `[open_time, close_time)`.
- Earliest `(event_time, sequence)` determines open; latest determines close.
- High/low compare every accepted trade price.
- Volume and trade count increase only after durable `market_trades` insertion succeeds.
- A late trade may correct a final candle; the corrected final candle is upserted and published again.

## Finalization

When a trade enters a later minute, every non-final candle whose close time is at or before the new minute watermark becomes final. High intervals are final only when every expected 1m bucket is present, contiguous and final.

## Gap repair

For a closed range with a known previous close, each missing 1m bucket becomes a final flat candle:

```text
open = high = low = close = previous close
volume = 0
trade_count = 0
```

Repaired 1m data is upserted, then affected 5m/1h buckets are recomputed from 1m source data. The operation is idempotent.

## Runtime checks

`make candle-quality` queries PostgreSQL and fails on:

- invalid OHLC ordering or negative volume/trade count;
- duplicate `(stream_id, trade_id)` facts;
- gaps between adjacent final candles of the same symbol and interval.

```bash
make candle-quality
```

Primary keys and unique constraints are also exercised by Testcontainers integration tests and backup/restore verification.
