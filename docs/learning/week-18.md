# Week 18：强平状态机

## 学习重点

- 维护保证金触发、bankruptcy price、liquidation distance。
- risk scan、幂等 job、强平订单、结算、审计证据和失败重试。
- 为什么触发价格只能用 mark price，为什么强平不是一个 SQL 更新。

## 代码导读

阅读 `internal/liquidation`、`cmd/risk-worker`、`liquidation_repository.go`、migration 00012 和 `skills/liquidation-review`。

## 动手与验收

重复扫描相同 `user + symbol + risk_version`，验证不会重复强平；注入事务失败证明不会留下半完成余额。

```bash
go test ./internal/liquidation ./internal/platform/postgres
BASE_URL=http://127.0.0.1:18080 make derivatives-check
```

复盘：画出 price update 到 liquidation audit 的完整时序。
