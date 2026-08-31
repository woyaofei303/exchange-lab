# Week 14：指数价格与标记价格

## 学习重点

- 指数价格的多源聚合、stale 判定与异常值拒绝。
- 标记价格的 premium clamp、确定性舍入与质量状态。
- last/index/mark 的用途差异；强平只依赖可信 mark。

## 代码导读

从 `internal/contractprice/price.go` 的纯函数开始，再读 worker、`internal/platform/postgres/price_repository.go`、migration 00008 和 `/v1/prices` handler。数据真值为 atoms + scale；HTTP 只给 decimal string。

## 动手与验收

新增一组“一个源异常、一个源过期、剩余源正常”的测试，再构造不足两路可信源的失败。

```bash
go test ./internal/contractprice
curl --fail --silent 'http://127.0.0.1:18080/v1/prices?symbol=BTCUSDT-PERP'
```

复盘：记录输入源、拒绝原因、最终指数、premium 上限和回滚方式。
