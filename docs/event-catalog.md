# Exchange Lab Event Catalog

All durable business events use this immutable envelope:

```json
{
  "version": 1,
  "id": "out_...",
  "type": "orders.accepted",
  "aggregate_type": "order",
  "aggregate_id": "ord_...",
  "correlation_id": "request-or-idempotency-key",
  "occurred_at": "2026-10-26T12:00:00Z",
  "data": {}
}
```

Rules: maximum encoded message 1 MiB; data must be an object; money/price/quantity/rate values are decimal strings; time is RFC 3339 UTC; unknown version/type is rejected and parked. Event IDs identify business facts, `Nats-Msg-Id` is a transport dedupe hint, and correlation IDs connect HTTP/Outbox/consumer evidence.

## ORDERS stream

- Subjects: `orders.>`
- Storage/retention: file, limits, 7 days, 1,000,000 messages, 1 GiB, 1 MiB/message.
- `orders.accepted`: a validated order and its margin reserve committed.
- `orders.cancel_requested`: cancellation and current remaining reserve release committed in v1.
- Durable `order-audit-v1`: explicit ACK, AckWait 5s, MaxDeliver 5, MaxAckPending 256 and bounded backoff.

## TRADES stream

- Subjects: `trades.>`
- Same bounded file-retention policy as ORDERS.
- The matching consumer emits trade events without changing the v1 envelope.
- `trades.executed`: matching-v1 committed one immutable maker/taker trade; payload includes decimal-string price/quantity and stable order/user references.

`position-v1` durable consumer 只订阅 `trades.executed`。payload 的 JSON 字段名是稳定 snake_case 契约；成交应用、仓位、保证金、费用/PnL 账本、dedupe 与 receipt 在一个 PostgreSQL 事务中提交。字段不兼容会进入 parking，不允许宽松猜测字段名。

## 内部风险事实

价格源、risk snapshot、liquidation job、funding settlement、insurance entry、conditional trigger、AI run 与 MCP tool audit 当前保存在 PostgreSQL，只追加或幂等更新，没有为了“事件化”而额外复制到 JetStream。未来只有出现独立消费者与明确重放需求时才新增 versioned subject。

## Replay and parking

Consumers treat delivery as at-least-once. `(consumer_name,event_id)` is inserted in the same PostgreSQL transaction as each side effect. Duplicate delivery ACKs without repeating the effect. Invalid envelopes and exhausted processing failures enter `parked_events` with a fixed reason code; raw errors are not stored.

Replay requires: identify a bounded event set, verify consumer/schema compatibility, reset or use a new named consumer, preserve original event IDs, observe duplicate/parking metrics, and reconcile side effects afterward. Never delete dedupe facts to “make replay work.”

每次重放后必须核对：订单状态、contract trades、position version、余额/账本净和、Outbox backlog 与 parked count。强平和资金费率使用自身业务幂等键，不通过删除这些键重新执行。
