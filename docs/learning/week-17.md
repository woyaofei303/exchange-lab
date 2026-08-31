# Week 17：全仓保证金

## 学习重点

- 全仓权益、跨仓位未实现盈亏、共享抵押和 maintenance 聚合。
- 风险版本如何避免过期决策；模式切换何时必须拒绝。
- 单事务快照与并发价格/成交更新的关系。

## 代码导读

阅读 `internal/risk`、risk repository 和 migration 00011，比较 isolated 与 cross 返回字段。

## 动手与验收

创建两个不同方向仓位，改变 mark price，验证共享 equity、maintenance 和 risk ratio。

```bash
go test ./internal/risk ./internal/platform/postgres
BASE_URL=http://127.0.0.1:18080 make derivatives-check
```

复盘：用一段文字解释全仓为何降低单仓隔离性，却提高资金利用率。
