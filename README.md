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

`make check` 运行格式、静态检查、单元测试、race 测试、构建和 Compose 配置验证。`make acceptance` 还会验证 Redis 故障时存活/就绪语义、恢复和 PostgreSQL 数据持久性。

```bash
make check
make acceptance
```

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
