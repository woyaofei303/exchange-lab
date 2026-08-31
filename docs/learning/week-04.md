# Week 04：PostgreSQL、迁移、事务与恢复

## 本周目标

让 PostgreSQL 从“健康检查依赖”升级为合约元数据的权威数据源，并建立可重复迁移、事务回滚、真实数据库集成测试和备份恢复闭环。

本周只建立 `users`、`instruments`、`candles`、`audit_logs` 和迁移历史。身份、K 线写入、账本和订单仍属于后续周次。

## 完成结果

- goose 嵌入式 SQL migration `00001_initial.sql`。
- 独立 `migrate` 二进制，支持 `up`、`down` 和 `version` 三种有界操作。
- Compose 的 API 等待 migration 成功后再启动。
- PostgreSQL 连接池有 min/max 和 connect timeout 配置。
- `/readyz` 同时验证连接和最低 schema version。
- `InstrumentRepository` 使用 atoms + scale 往返领域类型。
- `Transactor.WithinTx` 对 callback 失败执行 rollback。
- Testcontainers 启动真实 PostgreSQL 18.4，验证迁移、repository、取消和事务。
- `pg_dump` 备份恢复到独立数据库，再核对版本、表和种子合约。

## 建议阅读顺序

1. `migrations/00001_initial.sql`：先看数据库不变量如何落成约束。
2. `docs/data-dictionary.md`：理解每个字段的所有权和精度。
3. `internal/platform/postgres/instrument_repository.go`：看 atoms/scale 如何进入和离开数据库。
4. `internal/platform/postgres/transaction.go`：看 begin、callback、rollback 和 commit 边界。
5. `internal/platform/postgres/migrate.go` 与 `cmd/migrate/main.go`：看 library 与 one-shot command 的分工。
6. `internal/platform/postgres/store_integration_test.go`：从真实数据库验收理解完整生命周期。
7. `scripts/backup-restore-check.sh`：看为什么“成功生成备份”不等于“可以恢复”。

## 数据所有权

```text
PostgreSQL：users / instruments / candles / audit_logs 的唯一真值
Redis：后续只保存可重建缓存和临时协调状态
NATS：后续传递至少一次事件，不替代数据库事实
Go domain：在一次业务运算中维护类型和不变量
HTTP DTO：只负责公开传输格式
```

Week 04 的 `/v1/instruments` 已从 PostgreSQL 读取；ticker 暂时仍是 Week 03 的内存快照，后续由行情 worker 替换。

## 为什么数据库仍不用浮点

例如 BTCUSDT 合约的价格步长：

```text
price_step_atoms = 10
price_scale      = 2
decimal          = 0.10
```

数据库保存整数和 scale，repository 使用 `trading.FormatDecimal` 重建字符串，再由 `trading.NewInstrument` 重新验证。这样数据库、Go 和 HTTP 三层不会引入 `float64` 舍入。

## Migration 生命周期

```text
Docker Compose
    -> postgres healthy
    -> migrate up（失败则退出非 0）
    -> schema_migrations 写版本
    -> api 启动
    -> /readyz 检查 current >= required
```

常用命令：

```bash
make migrate
make migrate-version
```

`make migrate-down` 会回滚最近一个 migration，在当前版本会删除 Week 04 的应用表。它只应用于明确的本地回滚演练，运行前应先备份。

## 事务边界

`WithinTx` 的规则：

1. 使用带 deadline 的 context 开启事务。
2. callback 返回错误：rollback，并保留原始业务错误供 `errors.Is` 判断。
3. callback 返回 nil：commit。
4. begin、rollback 或 commit 自身失败时返回明确错误。

后续余额、账本、订单与 outbox 必须复用同样的事务边界，禁止先发消息再提交数据库。

## 索引验证

`instruments.symbol` 已是主键。强制关闭顺序扫描后执行 EXPLAIN，查询使用：

```text
Index Scan using instruments_pkey on instruments
  Index Cond: (symbol = 'BTCUSDT-PERP'::text)
  Filter: (status = 'active'::text)
```

数据量很小时 PostgreSQL 正常情况下可能选择顺序扫描，因此不能只看到 Seq Scan 就盲目增加重复索引。审计资源历史查询有独立复合索引；其他索引等真实查询和数据量出现后再用 EXPLAIN 决定。

## Testcontainers 与 Colima

Testcontainers 默认读取 `DOCKER_HOST`。当前 Mac 的 Docker CLI 使用 Colima context，但 shell 未导出该变量，所以直接运行原始 `go test -tags=integration` 会找不到 rootless socket。

`scripts/integration.sh` 会读取当前 Docker context；仅在 Colima socket 下设置：

```text
TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
```

因此统一使用：

```bash
make integration
```

## 备份与恢复

```bash
make backup-restore-check
```

脚本执行：

1. `pg_dump --format=custom` 备份当前开发数据库。
2. 创建名称受限的独立临时数据库。
3. `pg_restore --exit-on-error` 恢复。
4. 核对 schema version、五张表与 `BTCUSDT-PERP`。
5. 删除临时数据库和临时 dump。

它不会删除原数据库，也不会把密码或完整 DSN写入日志。

## TDD 记录

1. `FormatDecimal` 测试先编译失败，再实现 atoms + scale 格式化。
2. PostgreSQL integration test 先因存储包和 Testcontainers 缺失失败，再添加最小实现。
3. Config 测试先因 pool limits/database timeout 缺失失败，再扩展配置。
4. Composite market reader 测试先找不到构造器，再隔离 instrument/ticker 所有权。
5. migration CLI 测试先找不到受限 direction parser，再只允许 up/down/version。
6. Compose contract 测试先接受无 migration gate 的配置，再强制 migration 成功依赖。
7. schema readiness 测试先找不到 checker，再验证 down 后 not ready、up 后 ready。
8. `make backup-restore-check` 先无目标失败，再实现真实恢复验收。
9. Testcontainers 首次在 Colima provider 发现阶段失败；确认根因后用官方 socket 配置重跑通过。

## 练习

1. 在 migration 中临时删除 `CHECK (high_atoms >= low_atoms)`，写入错误 Candle，观察数据库为何不再保护领域不变量，然后还原。
2. 给 `instruments` 保存非法 symbol，比较领域构造器错误和数据库 constraint 错误。
3. 在事务 callback 中插入两行后返回自定义错误，确认两行都不存在。
4. 执行 `EXPLAIN (ANALYZE, BUFFERS)` 前先生成足量测试数据，再比较主键查询与无索引查询。
5. 修改本地 migration 副本制造语法错误，确认 migrate container 失败时 API 不启动。

## AI 审查清单

- migration down 是否删除了超出该 migration 所有权的对象。
- nullable、唯一约束、外键和删除策略是否明确。
- 金额、价格、数量字段是否仍为 atoms + scale，而不是 float。
- SQL 是否接收 context 和 timeout。
- 事务失败是否一定 rollback，消息是否在 commit 前错误发送。
- 日志和错误是否泄露密码或完整 DSN。
- 新索引是否有真实查询与 EXPLAIN 证据。
- 备份是否真的在独立数据库恢复并核对。

## 验收命令

```bash
make check
make integration
make migrate-version
make backup-restore-check
API_HOST_PORT=18080 BASE_URL=http://127.0.0.1:18080 make up smoke acceptance
```

本机 8080 已由用户的 nginx 占用，所以运行验收使用项目原有的 host-port override；未停止或修改 nginx。

## 验收结果

- [x] migration up/down/重复 up 在全新 PostgreSQL 18.4 中通过。
- [x] repository 精度往返、context 取消和事务 rollback 通过。
- [x] schema migration 落后时 readiness 失败，恢复后成功。
- [x] `make check` 与 race tests 通过。
- [x] Docker 镜像同时构建 API 与 migration binary。
- [x] migration gate、四服务健康、smoke 与故障恢复 acceptance 通过。
- [x] `pg_dump` 已恢复到独立数据库并完成数据核对。
