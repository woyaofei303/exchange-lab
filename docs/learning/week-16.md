# Week 16：逐仓保证金

## 学习重点

- initial margin、maintenance margin、leverage tier 和 margin ratio。
- 逐仓保证金与用户 available/locked 余额之间的转移。
- 调整保证金的幂等、行锁、风险版本和事务回滚。

## 代码导读

从 `internal/margin` 的纯计算开始，追到 risk repository、migration 00010 和 margin HTTP endpoint。

## 动手与验收

增加“减少后低于维护保证金”“同一幂等键参数冲突”“并发调整”的测试。

```bash
go test ./internal/margin ./internal/platform/postgres
BASE_URL=http://127.0.0.1:18080 make derivatives-check
```

复盘：列出逐仓仓位可损失的边界和余额不变量。
