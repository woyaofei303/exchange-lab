# Week 19：资金费率

## 学习重点

- premium、费率 clamp、结算窗口、long/short 支付方向。
- 精确乘除和舍入余数；模拟单边持仓的 system offset。
- settlement 幂等、账本零和和零费率不产生空 entry。

## 代码导读

先读 `internal/funding/funding_test.go`，再读 `funding.go`、risk repository 的结算实现、`cmd/funding-worker` 和 migration 00013。

## 动手与验收

覆盖正费率、负费率、零费率、重复窗口和单边持仓五种用例，并运行账本对账。

```bash
go test ./internal/funding ./internal/platform/postgres
BASE_URL=http://127.0.0.1:18080 make derivatives-check
```

复盘：证明每个结算窗口所有 ledger entries 之和为零。
