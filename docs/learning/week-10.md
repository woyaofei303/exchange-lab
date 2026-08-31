# 第 10 周：双向账本、余额与模拟入金

## 本周目标

通过一个可运行的模拟 USDT 水龙头理解：固定精度金额、借贷守恒、数据库事务、行锁、Serializable 隔离、幂等键、物化余额与不可变账本的关系。

## 代码阅读顺序

1. `internal/ledger/ledger.go`：先看 `NewTransaction` 如何拒绝不守恒、混合资产、零 entry 和溢出，再看 `Balance` 状态转换。
2. `migrations/00004_ledger.sql`：区分账户、余额快照、账本事务、entry 和业务幂等表。
3. `internal/platform/postgres/ledger_repository.go`：沿 `Faucet` 阅读同一事务中的 advisory lock、余额行锁、entry 守恒复查、快照、幂等与审计。
4. `internal/httpserver/ledger.go`：观察 HTTP 边界只收发十进制字符串，认证身份来自 server-side session。
5. `scripts/ledgercheck.sh`：阅读真实服务的重复请求与 SQL 对账证据。

## 动手练习

```bash
go test ./internal/ledger ./internal/httpserver
make integration
API_HOST_PORT=18080 BASE_URL=http://127.0.0.1:18080 make ledger-check
```

尝试增加一条只有单边 entry 的单元测试，确认它先失败；再阅读构造器为何拒绝。随后并发发送相同幂等键，验证只有一个账本事务。

## 完成标准

- 领域测试覆盖守恒、溢出、余额不足和锁定/释放。
- PostgreSQL 集成测试覆盖首次入金、完全相同重放、冲突重放和对账。
- API 不传输 atoms 数字或 session token；金额始终是八位小数字符串。
- 真实 Compose 验收证明重复请求不重复入金，所有账本事务净和为零。
