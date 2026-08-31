# Exchange Lab

`exchange-lab` 是一个教学级、纯模拟的 USDT 本位永续合约系统，用于学习 Go 全栈、交易系统工程、事件驱动架构和只读 AI 协作。项目不连接真实资金，不提供投资建议。

当前交付版本为 `v1.0.0`，数据库迁移版本为 `16`。仓库已完成本地/参考环境交付；真实公网 staging 仍需要独立的 Linux 主机、域名、DNS 和部署密钥。

## 能力范围

当前实现包括：

- 行情：确定性 WebSocket simulator、序列校验、NATS 分发、Redis 最新行情缓存、1m/5m/1h K 线。
- 账户：注册、登录、Opaque Session、RBAC、审计、模拟 USDT 入金和复式账本。
- 交易：限价单、Transactional Outbox、JetStream、确定性价格时间优先撮合、成交结算和单向仓位。
- 风控：指数价/标记价、逐仓/教学级全仓、杠杆、强平、资金费率、保险基金、简化 ADL 和 reduce-only 条件单。
- 前端：React 合约终端、历史/实时 K 线、订单、仓位、保证金和风险展示。
- AI：基于服务端证据的只读风险解释、确定性本地回退、只读 MCP 工具和审计。
- 工程：OpenTelemetry、Prometheus、Grafana、测试/故障演练、生产 Compose、安全检查、备份、发布和回滚脚本。

明确不包含真实充值、提现、托管、KYC、法币、真实清算、自动交易 AI、生产级多分区撮合或多地域高可用。完整边界见 [产品范围](docs/product-scope.md) 与 [已知限制](docs/known-limitations.md)。

## 核心约束

- 价格、数量、金额、保证金和 PnL 的真值必须使用整数 atoms 与显式 scale，禁止使用二进制浮点。
- 账本交易至少包含两条分录且总和为零；重复业务事件不得产生第二次资金副作用。
- PostgreSQL 保存交易与审计真值；Redis 只保存可重建缓存和短期安全状态；JetStream 负责有限保留的耐久交付。
- 风控和强平只使用带 freshness 证据的标记价格，不以最新成交价替代。
- AI/MCP 只能读取服务端已经计算的事实，不能下单、撤单、改杠杆、调保证金或改余额。

完整运行结构、模块边界和一致性策略见 [架构说明](docs/architecture.md)。

## 快速启动

需要：

- Go `1.26.6`
- Node.js、npm
- Docker 与 Docker Compose `2.17.0+`
- `make`、POSIX shell、`curl`、`mktemp`

首次启动：

```bash
cp .env.example .env
make web-install
make up
make smoke
```

`make up` 会构建 Web 与 Go 镜像，等待 PostgreSQL/Redis/NATS 健康，执行迁移，再启动 workers、API 和 Web 终端。

默认端口只绑定到本机 loopback：

```text
Web terminal       http://127.0.0.1:3000
API                http://127.0.0.1:8080
PostgreSQL         127.0.0.1:5432
Redis              127.0.0.1:6379
NATS client        127.0.0.1:4222
NATS monitoring    http://127.0.0.1:8222
Candle worker      http://127.0.0.1:8091
Event worker       http://127.0.0.1:8092
Price worker       http://127.0.0.1:8093
Risk worker        http://127.0.0.1:8094
Funding worker     http://127.0.0.1:8095
```

如果 `8080` 已被占用：

```bash
API_HOST_PORT=18080 BASE_URL=http://127.0.0.1:18080 make up smoke acceptance
```

`POSTGRES_DSN` 必须与 `POSTGRES_USER`、`POSTGRES_DB`、`POSTGRES_PASSWORD` 一致。密码含 `@`、`:`、`/` 等 URL 保留字符时，DSN userinfo 必须使用百分号编码。不要提交真实密码或在日志中打印 DSN、Token、私钥。

## API

常用只读检查：

```bash
curl --fail --silent http://127.0.0.1:8080/healthz
curl --fail --silent http://127.0.0.1:8080/readyz
curl --fail --silent http://127.0.0.1:8080/v1/instruments
curl --fail --silent 'http://127.0.0.1:8080/v1/tickers?symbol=BTCUSDT-PERP'
curl --fail --silent 'http://127.0.0.1:8080/v1/prices?symbol=BTCUSDT-PERP'
```

`/healthz` 只表示 API 进程存活；`/readyz` 会检查 PostgreSQL schema、Redis 和 NATS。任一依赖未就绪时返回 `503`。

所有公开金融小数使用字符串。响应包含 `X-Request-ID`，错误使用统一、脱敏的 envelope：

```json
{"error":{"code":"invalid_request","message":"invalid request","request_id":"client-123"}}
```

完整路由、参数、认证和响应契约以 [openapi.yaml](openapi.yaml) 为准。

## 验证

本地静态检查、单元测试、race、构建、前端测试和项目合约：

```bash
make check
```

依赖真实 PostgreSQL/Redis/NATS 或完整运行栈的检查按需执行：

```bash
docker compose up -d --wait postgres redis nats
make integration
make migrate-version
make backup-restore-check
make acceptance
make web-e2e
make auth-check
make ledger-check
make order-check
make event-check
make matching-check
make derivatives-check
make fuzz-smoke
make security-check
make release-check
```

测试分层、环境要求和门禁顺序见 [测试策略](docs/testing-strategy.md)。

## 文档

文档统一入口为 [docs/README.md](docs/README.md)，其中按以下用途分类：

- 当前事实：产品范围、架构、数据字典、ERD、OpenAPI 与已知限制。
- 设计决策：固定精度、账本、Outbox、JetStream、撮合、标记价、保证金、强平、模块化单体和只读 AI。
- 领域与协议：行情、K 线、事件、撮合、风险公式、缓存和前端调用链。
- 运维：SLO、安全、威胁模型、测试、性能、发布、备份、故障和回滚。
- 学习历史：28 周学习路线与每周记录；这些是阶段快照，不替代当前架构和运行文档。
- 求职材料：项目演示、简历和面试表达，与运行手册分开维护。

## 停机与重置

日常停机保留 PostgreSQL、Redis 和 NATS 数据卷：

```bash
make down
```

以下命令会永久删除本项目的本地数据卷，只在明确需要全量重置时运行：

```bash
docker compose down --volumes
```

## 许可证与安全边界

所有余额、订单、仓位和资金流均为模拟数据。AI 工具保持只读，除非后续经过单独评审的设计明确授权一个狭窄写能力。
