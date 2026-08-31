# Week 20：保险基金、简化 ADL 与条件单

## 学习重点

- bankruptcy loss、insurance debit、剩余 shortfall 与 ADL 候选证据。
- mark/last 触发源、TP/SL 方向、reduce-only 和重复触发幂等。
- 教学模型与生产交易所风控模型的边界。

## 代码导读

阅读 `internal/insurance`、liquidation repository、risk repository 和 migration 00014；再从 HTTP 查看 liquidation、insurance 与 conditional order。

## 动手与验收

构造基金足够/不足和条件单恰好触发/不触发的边界测试。

```bash
go test ./internal/insurance ./internal/liquidation ./internal/platform/postgres
BASE_URL=http://127.0.0.1:18080 make derivatives-check
```

复盘：明确当前 ADL 只做排序与审计证据，不声称实现真实交易所的完整自动减仓撮合。
