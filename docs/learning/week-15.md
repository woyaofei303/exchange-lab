# Week 15：仓位与 PnL

## 学习重点

- 单向持仓下的开仓、同向加仓、部分减仓、平仓和反向开仓。
- 加权 entry、realized/unrealized PnL、手续费累计、reduce-only。
- `trades.executed` 的幂等消费与 PostgreSQL 仓位快照。

## 代码导读

先读 `internal/position/position_test.go`，再读 `position.go` 和 `position_repository.go`。特别关注成交 JSON 契约、consumer dedupe 和同事务更新。

## 动手与验收

手算 long/short 各一组 open → add → partial close → flip，用测试证明结果。

```bash
go test ./internal/position ./internal/platform/postgres
BASE_URL=http://127.0.0.1:18080 make derivatives-check
```

复盘：为什么仓位快照可更新，但成交与账务事实只追加。
