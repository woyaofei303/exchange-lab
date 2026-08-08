# Exchange Lab

用于通过代码学习 Go 全栈、交易系统工程和 AI 协作的模拟 USDT 本位永续合约项目。本项目不连接真实资金。

## Week 01 Foundation

- Go 1.26.5 HTTP API
- PostgreSQL 18.4
- Redis 8.4.1
- NATS 2.12.4 with JetStream
- Docker Compose and GitHub Actions
- `/healthz` process liveness and `/readyz` dependency readiness

## Quickstart

需要安装 Go 1.26.5、Docker、Docker Compose v2.17.0+、`make`、POSIX shell、`curl` 和 `mktemp`。可先检查 Compose 版本：

```bash
docker compose version
```

首次启动时复制本地环境配置，启动四个服务并等待它们全部健康，再运行冒烟检查：

```bash
cp .env.example .env
make up
make smoke
```

`POSTGRES_DSN` 是 API 使用的权威连接配置；`POSTGRES_USER`、`POSTGRES_DB` 和 `POSTGRES_PASSWORD` 仅用于初始化官方 PostgreSQL 容器。覆盖这些值时必须保持用户名、数据库名和密码一致。`POSTGRES_PASSWORD` 保留原始密码；如果密码含 `@`、`:`、`/` 等 URL 保留字符，`POSTGRES_DSN` 的 userinfo 中必须使用百分号编码后的密码。例如，原始密码中的 `@` 要写成 `%40`。不要把真实密码提交到仓库或打印到日志。

默认所有宿主机端口只绑定到 loopback：

- API: `127.0.0.1:8080`
- PostgreSQL: `127.0.0.1:5432`
- Redis: `127.0.0.1:6379`
- NATS client: `127.0.0.1:4222`
- NATS monitoring: `127.0.0.1:8222`

如果本机的 `8080` 端口已被占用，可以只改 API 宿主机端口；同一条命令中的 Compose 和验收步骤会共享该设置：

```bash
API_HOST_PORT=18080 BASE_URL=http://127.0.0.1:18080 make up smoke acceptance
```

## Endpoints

`/healthz` 只表示 API 进程存活；`/readyz` 会检查 PostgreSQL、Redis 和 NATS，任一依赖不可用时返回 HTTP `503`。

```bash
curl --fail --silent http://127.0.0.1:8080/healthz
curl --fail --silent http://127.0.0.1:8080/readyz
```

正常响应示例：

```json
{"status":"ok"}
{"status":"ready","checks":{"nats":"ok","postgres":"ok","redis":"ok"}}
```

## Verification

`make check` 运行格式、POSIX shell 语法、静态检查、单元测试、race 测试、构建、Compose 合约和 Docker 上下文验证。依赖服务健康并暴露在默认 loopback 端口时，`make integration` 会以 race 模式运行带 `integration` build tag 的真实依赖检查。`make acceptance` 还会验证 Redis 故障、NATS 冷启动故障及自动恢复，以及 PostgreSQL 重建后的数据持久性。

```bash
make check
docker compose up -d --wait postgres redis nats
make integration
make acceptance
```

## Dependency Decisions

Week 1 只建立本地基础设施、连接生命周期和可观测就绪语义，不引入撮合、资金、托管或真实交易写入。

### PostgreSQL + pgx/pgxpool

- 用途：PostgreSQL 是后续交易域持久数据的主存储；API 使用 `pgxpool` 管理连接并执行 `/readyz` 的 `Ping`。
- 数据所有权：数据库数据归 PostgreSQL 服务所有，保存在 `postgres_data` 命名卷；API 不复制或暗中迁移所有权。
- 失败模式：启动时 DSN 结构无效或与 pgx 不兼容，API 会以固定脱敏配置错误拒绝启动；配置有效但运行时 PostgreSQL 不可用时，API 进程仍存活，`/healthz` 返回 `200`，`/readyz` 将 `postgres` 标为 `error` 并返回 `503`。
- 回滚路径：回退 API/pgx 或 PostgreSQL 镜像版本时默认保留 `postgres_data`，先做兼容性验证或备份；日常回滚只用 `docker compose down`，不得用 `--volumes`，除非明确接受销毁数据。

### Redis + go-redis

- 用途：Redis 为后续短生命周期缓存和协调状态提供服务；API 使用 `go-redis`，并通过 `PING` 暴露就绪状态。
- 数据所有权：Redis/AOF 数据归 Redis 服务所有，保存在 `redis_data` 命名卷；Week 1 不把 Redis 当作 PostgreSQL 的权威副本。
- 失败模式：启动时 `REDIS_ADDR` 无效，API 会以固定脱敏配置错误拒绝启动；配置有效但运行时 Redis 不可用时，`/healthz` 仍返回 `200`，`/readyz` 仅将 `redis` 标为 `error` 并返回 `503`；恢复后无需重启 API 即可重新就绪。
- 回滚路径：回退 API/go-redis 或 Redis 镜像时默认保留 `redis_data`；若缓存格式不兼容，先停用相关功能或做受控清理，不以删除全部 Compose 数据卷作为默认手段。

### NATS JetStream + nats.go

- 用途：NATS JetStream 为后续事件传递和耐久消息提供基础；API 使用 `nats.go`，就绪检查读取 JetStream account 信息。
- 数据所有权：JetStream 持久消息归 NATS 服务所有，保存在 `nats_data` 命名卷；Week 1 尚未创建业务 stream 或发布交易事件。
- 失败模式：启动时 `NATS_URL` 结构无效，API 会以固定脱敏配置错误拒绝启动；配置有效但 NATS 在 API 冷启动时不可用不会终止进程，客户端以有界间隔无限重连，`/healthz` 返回 `200`，`/readyz` 仅将 `nats` 标为 `error` 并返回 `503`，连接恢复后自动返回就绪。
- 回滚路径：回退 API/nats.go 或 NATS 镜像时默认保留 `nats_data`，先验证 JetStream 存储兼容性；不得把 `docker compose down --volumes` 当作普通回滚步骤。

## Shutdown and Reset

日常停机使用以下命令。它会停止并移除容器和网络，但保留 PostgreSQL、Redis 和 NATS 的命名数据卷：

```bash
make down
```

### Destructive reset

警告：以下命令会永久删除本项目的 PostgreSQL、Redis 和 NATS 命名数据卷及其中的所有本地数据。只在明确需要全量重置时运行：

```bash
docker compose down --volumes
```

## Safety Boundary

All balances are simulated. No deposit, withdrawal, custody, or real-money integration is in scope. AI tools remain read-only unless a separately reviewed design explicitly authorizes a narrow write operation.
