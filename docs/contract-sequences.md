# 永续合约关键序列

## 下单、撮合与仓位

```mermaid
sequenceDiagram
  participant U as User
  participant A as API
  participant P as PostgreSQL
  participant N as JetStream
  participant E as event-worker
  U->>A: POST /v1/orders + Idempotency-Key
  A->>P: Serializable: risk check + freeze + order + Outbox
  E->>P: claim Outbox
  E->>N: orders.accepted
  N->>E: matching-v1
  E->>P: dedupe + price-time match + trade + trade Outbox
  E->>N: trades.executed
  N->>E: position-v1
  E->>P: dedupe + position + margin + fee/PnL ledger
  A-->>U: GET positions/risk
```

## 资金费

```mermaid
sequenceDiagram
  participant F as funding-worker
  participant P as PostgreSQL
  F->>P: lock funding timestamp and position snapshots
  P-->>F: mark/index, sizes, versions
  F->>F: exact signed payment + cap
  F->>P: one transaction: rate + settlements + ledger
  Note over F,P: residual one-sided exposure is offset by the simulated insurance fund
  F->>P: mark settled; repeat returns existing result
```

## 强平

```mermaid
sequenceDiagram
  participant R as risk-worker
  participant P as PostgreSQL
  R->>P: read fresh mark + position + MM + risk version
  R->>R: Evaluate liquidation snapshot
  R->>P: insert idempotent liquidation job
  R->>P: lock job/position/margin/balance
  R->>P: reduce-only close + PnL ledger + margin release
  R->>P: insurance loss coverage, optional ADL candidates
  R->>P: completed event and risk-version increment
```
