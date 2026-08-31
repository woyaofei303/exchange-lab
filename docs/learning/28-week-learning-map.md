# Exchange Lab 28 周学习与交付路线

> 本目录记录项目按周演进时的学习上下文，其中“本周”“后续”是历史阶段描述。当前能力与运行事实以 `docs/README.md`、架构、OpenAPI、迁移和实际代码为准。

目标不是“看完一套 Go 教程”，而是通过一条可运行的 USDT 本位永续合约链路，掌握 Go 后端、数据库、消息系统、React、AI 工具化、Docker、可观测性和发布恢复。项目只使用模拟资金，不连接充值、提现、托管或真实交易。

建议每周投入 12–15 小时：约 30% 阅读和画图，50% 代码与测试，20% 复盘。每周都执行“读代码 → 修改一个小功能 → 写失败测试 → 实现 → 跑验收 → 写复盘”，不要只复制代码。

## 通用学习循环

1. 阅读本周 `docs/learning/week-XX.md` 和对应 ADR/专题文档。
2. 从测试文件开始，写出输入、输出、不变量和失败模式。
3. 只实现让测试通过的最小改动；交易金额、价格、数量、保证金、PnL 禁止使用 `float64` 作为真值。
4. 跑本周命令并观察真实 PostgreSQL、Redis、NATS、浏览器或容器行为。
5. 在自己的学习笔记中回答：数据归谁、失败会怎样、如何重放、如何回滚。

## Week 01：Go 与容器化开发底座

- 学习：Go module、package、context、`net/http`、优雅关闭、Docker Compose、健康检查。
- 代码：`cmd/api`、`internal/app`、`internal/health`、`compose.yaml`。
- 练习：给 readiness 新增一个可注入检查器，并先写 200/503 测试。
- 验收：`make check`、`make up`、`make smoke`。
- 产出：能解释 liveness 与 readiness 的差别，以及依赖失败为什么不应泄露原始错误。

## Week 02：精确数值与交易领域类型

- 学习：整数 atoms、scale、decimal string、溢出、tick/step/min-notional。
- 代码：`internal/trading`、`internal/fixedmath`、ADR-001。
- 练习：新增边界用例，证明非法精度、数量步长和溢出会被拒绝。
- 验收：`make domain-check && go test ./internal/trading ./internal/fixedmath`。
- 产出：能说明为什么账务真值不能使用二进制浮点数。

## Week 03：HTTP API 与 OpenAPI 契约

- 学习：路由、状态码、统一错误、请求 ID、超时、JSON 严格解码、契约优先。
- 代码：`internal/httpserver`、`openapi.yaml`。
- 练习：先在 OpenAPI 和 handler 测试中增加一个只读字段，再实现它。
- 验收：`make openapi-check && go test ./internal/httpserver`。
- 产出：能从接口契约追踪到 handler、service 和 repository。

## Week 04：PostgreSQL、迁移与事务

- 学习：关系模型、约束、索引、migration、连接池、事务隔离、备份恢复。
- 代码：`migrations`、`internal/platform/postgres`、`docs/data-dictionary.md`。
- 练习：为一个只追加事实表补索引，并用集成测试验证迁移 up/down。
- 验收：`make integration && make backup-restore-check`。
- 产出：能说明 schema、业务事实和迁移历史各自的所有权。

## Week 05：Redis 缓存与安全状态

- 学习：cache-aside、TTL、版本化 key、Lua 原子操作、fail-open 与 fail-closed。
- 代码：`internal/platform/rediscache`、`internal/market`、`docs/cache-catalog.md`。
- 练习：模拟 Redis 清空和宕机，比较行情缓存降级与登录限流失败的不同语义。
- 验收：`make integration && make acceptance`。
- 产出：能区分可重建缓存和必须失败关闭的安全状态。

## Week 06：WebSocket 行情可靠性

- 学习：snapshot/delta、sequence、gap、heartbeat、单 reader、背压、指数退避。
- 代码：`internal/marketdata`、`cmd/market-simulator`、`cmd/market-worker`。
- 练习：构造重复、乱序、缺口和超大 frame，观察状态机如何拒绝或重连。
- 验收：`make market-check && go test ./internal/marketdata`。
- 产出：能解释“连接成功”为什么不等于“行情连续且可信”。

## Week 07：事件时间 K 线流水线

- 学习：event time、watermark、迟到事件、幂等、OHLC、rollup、gap repair。
- 代码：`internal/candle`、`cmd/candle-worker`、migration 00002。
- 练习：插入迟到成交和重复成交，验证 open/close、volume 和 trade count。
- 验收：`make candle-check && make candle-quality`。
- 产出：能从原始 trade 追到 1m、5m、1h K 线和实时推送。

## Week 08：React 合约行情终端

- 学习：React/TypeScript、TanStack Query、运行时解码、KLineChart、WS 重连状态。
- 代码：`web/src`、`cmd/web-gateway`。
- 练习：新增一个行情派生指标，保留 decimal string 真值边界，并补前端测试。
- 验收：`make web-test && make web-build && make web-e2e`。
- 产出：能解释前端何处允许为展示转换 number，何处必须保留字符串。

## Week 09：身份、会话与 RBAC

- 学习：Argon2id、opaque token、token hash、cookie、CSRF、限流、权限最小化。
- 代码：`internal/identity`、`internal/httpserver/identity.go`、migration 00003。
- 练习：验证登录失败、刷新重放、登出撤销、普通用户访问管理接口。
- 验收：`make auth-check && go test ./internal/identity ./internal/httpserver`。
- 产出：能画出浏览器、API、Redis 与 PostgreSQL 的认证信任边界。

## Week 10：复式账本与余额快照

- 学习：账户、事务、entry、借贷平衡、幂等、对账、反向冲正。
- 代码：`internal/ledger`、ledger repository、ADR-002。
- 练习：实现一条错误的不平衡交易测试，确保在进入数据库前被拒绝。
- 验收：`make ledger-check && make backup-restore-check`。
- 产出：能证明任意一笔业务账务在同资产内净和为零。

## Week 11：订单事务与 Transactional Outbox

- 学习：下单校验、保证金冻结、Serializable、幂等键、Outbox 双写问题。
- 代码：`internal/order`、order repository、ADR-003。
- 练习：模拟事务任一步失败，证明订单、余额、账本、Outbox 一起回滚。
- 验收：`make order-check`。
- 产出：能解释为什么不能先写数据库、再“尽力”发消息。

## Week 12：JetStream 与幂等消费

- 学习：stream、subject、durable consumer、ACK/NAK、redelivery、parking、replay。
- 代码：`internal/eventing`、`cmd/event-worker`、`docs/event-catalog.md`。
- 练习：重复投递同一 event ID，证明副作用只发生一次；再测试坏 envelope 进入 parking。
- 验收：`make event-check`。
- 产出：能制定不删除 dedupe 事实的安全重放流程。

## Week 13：确定性撮合引擎

- 学习：价格优先、时间优先、maker price、部分成交、订单簿快照、确定性回放。
- 代码：`internal/matching`、matching repository、`docs/matching-engine.md`。
- 练习：增加 FIFO、部分成交、取消与 replay 属性测试，运行 benchmark。
- 验收：`make matching-check && go test -race ./internal/matching`。
- 产出：能从两笔订单追踪到成交、订单状态、book、offset 和 trade event。

## Week 14：指数价格与标记价格

- 学习：多源价格、中位数、陈旧源、异常值、premium clamp、标记价格安全边界。
- 代码：`internal/contractprice`、`cmd/price-worker`、migration 00008。
- 练习：构造单源异常、两源过期和溢价越界测试，观察价格质量状态。
- 验收：`go test ./internal/contractprice && curl --fail --silent 'http://127.0.0.1:18080/v1/prices?symbol=BTCUSDT-PERP'`。
- 产出：能解释为何强平依据 mark price，而不是容易被瞬时操纵的 last price。

## Week 15：仓位、成交应用与 PnL

- 学习：单向持仓、加仓均价、减仓、反向开仓、已实现/未实现 PnL、reduce-only。
- 代码：`internal/position`、position repository、migration 00009。
- 练习：覆盖 long/short 的 open、increase、partial close、full close、flip。
- 验收：`go test ./internal/position ./internal/platform/postgres && make derivatives-check`。
- 产出：能手工计算一笔仓位的 entry、size、realized PnL 与 unrealized PnL。

## Week 16：逐仓保证金

- 学习：leverage、initial margin、maintenance margin、margin ratio、追加/减少保证金。
- 代码：`internal/margin`、risk repository、migration 00010。
- 练习：尝试把逐仓保证金减少到维护保证金以下，确认事务拒绝且余额不变。
- 验收：`go test ./internal/margin && make derivatives-check`。
- 产出：能说明仓位保证金、可用余额、冻结余额与风险快照的关系。

## Week 17：全仓保证金

- 学习：共享权益、跨仓位未实现盈亏、风险版本、并发一致性、模式切换限制。
- 代码：`internal/risk`、risk repository、migration 00011。
- 练习：创建两个仓位，改变其中一个 mark price，验证全仓权益和风险率重算。
- 验收：`go test ./internal/risk ./internal/platform/postgres && make derivatives-check`。
- 产出：能比较逐仓爆仓边界与全仓共享抵押的风险差异。

## Week 18：强平状态机

- 学习：维护保证金、强平触发、bankruptcy price、幂等 job、审计证据、补偿失败。
- 代码：`internal/liquidation`、`cmd/risk-worker`、liquidation repository、migration 00012。
- 练习：重复运行同一 risk version 的扫描，证明只生成/执行一个强平任务。
- 验收：`go test ./internal/liquidation ./internal/platform/postgres && make derivatives-check`。
- 产出：能按“价格 → 风险快照 → job → 强平单 → 账本/保险 → 审计”讲完整链路。

## Week 19：资金费率与结算

- 学习：premium、clamp、结算窗口、long/short transfer、舍入余数、系统对手方、零和。
- 代码：`internal/funding`、`cmd/funding-worker`、migration 00013。
- 练习：测试正/负/零费率、重复结算与单边模拟持仓，核对 ledger 净和为零。
- 验收：`go test ./internal/funding ./internal/platform/postgres && make derivatives-check`。
- 产出：能解释资金费率不是交易手续费，也不是平台凭空创造资金。

## Week 20：保险基金、简化 ADL 与条件单

- 学习：bankruptcy loss、insurance coverage、ADL 排名证据、TP/SL trigger、reduce-only。
- 代码：`internal/insurance`、risk/liquidation repository、migration 00014。
- 练习：构造保险不足场景，检查 ADL 候选按盈利与杠杆排序且保存证据。
- 验收：`go test ./internal/insurance ./internal/liquidation && make derivatives-check`。
- 产出：能明确本项目 ADL 是教学简化实现，不把它包装成生产交易所算法。

## Week 21：完整合约交易终端

- 学习：认证态、委托、仓位、保证金、强平距离、资金费率、错误/恢复状态。
- 代码：`web/src/TradingPanel.tsx`、`web/src/api.ts`、`web/tests/contract-flow.spec.ts`。
- 练习：双用户挂单成交，刷新浏览器恢复仓位，再做 reduce-only 平仓。
- 验收：`BASE_URL=http://127.0.0.1:18080 make web-e2e`。
- 产出：浏览器可完整演示开仓、风险变化和关闭仓位，而不是只有静态页面。

## Week 22：只读 AI 风险助手

- 学习：结构化输出、工具调用、证据绑定、fallback、prompt injection、评测集。
- 代码：`internal/ai`、`internal/httpserver/ai.go`、migration 00015。
- 练习：让 30 个正常/边界/恶意样本通过 schema、引用和只读约束；断开模型验证确定性降级。
- 验收：`go test ./internal/ai && make derivatives-check`。
- 产出：能证明 AI 只解释已授权事实，不能下单、改保证金或访问他人数据。

## Week 23：MCP、项目 Skills 与 AI 辅助研发

- 学习：tool schema、最小权限、审计、超时、数据裁剪、Skill 渐进式说明。
- 代码：`internal/readtools`、`cmd/mcp-server`、`skills/*`、migration 00016。
- 练习：调用只读工具并检查 `tool_audit_logs`；使用项目 Skill 完成一次小需求和一次 review。
- 验收：`go test ./internal/readtools ./internal/ai && make derivatives-check`。
- 产出：形成“AI 读规则/工具 → 生成修改 → 测试证明 → 人审”的开发闭环。

## Week 24：可观测性与 SLO

- 学习：RED 指标、业务不变量指标、trace、dashboard、alert、SLO/错误预算。
- 代码：`internal/observability`、`observability/*`、`compose.observability.yaml`。
- 练习：注入 NATS 故障，观察 readiness、告警和 trace；按 runbook 定位而非猜测。
- 验收：启动 observability overlay，检查 Prometheus targets、Grafana provisioning 和 Tempo。
- 产出：能从请求 ID/trace ID 追到 HTTP、数据库、消息和业务事件。

## Week 25：测试金字塔、性能与故障演练

- 学习：unit/integration/E2E/property/fuzz/race/benchmark/load/chaos 的职责边界。
- 代码：`scripts/fuzz-smoke.sh`、`load/k6-contract.js`、`scripts/fault-injection.sh`。
- 练习：先定义目标与机器环境，再跑 fuzz、benchmark、k6 和 NATS 故障；不要只报一个脱离上下文的 QPS。
- 验收：`make fuzz-smoke && make load-test && BASE_URL=http://127.0.0.1:18080 make fault-injection`。
- 产出：`output-tdd/performance` 下可复查证据和一份瓶颈分析。

## Week 26：生产化容器、网络与安全基线

- 学习：multi-stage、non-root、read-only FS、cap drop、Docker secrets、TLS、最小暴露、SCA。
- 代码：`Dockerfile`、`compose.prod.yaml`、`deploy/Caddyfile`、`scripts/security-check.sh`。
- 练习：渲染生产 Compose，确认只有 Caddy 暴露宿主机端口，API 密钥只通过 secret file 注入。
- 验收：`make security-check && make release-check`。
- 产出：能解释镜像、secret、内部网络、反向代理和数据库的信任边界与回滚方式。

## Week 27：CI/CD、备份与回滚

- 学习：immutable tag、质量门禁、镜像扫描、环境审批、迁移策略、健康门禁、备份恢复、rollback。
- 代码：`.github/workflows`、`deploy/release.sh`、`deploy/rollback.sh`、`scripts/backup*.sh`。
- 练习：用两个不可变 tag 做一次发布检查和回滚演练；验证备份能恢复到全新数据库。
- 验收：`make backup-restore-check && make release-check`，并检查 workflow YAML。
- 产出：明确“回滚二进制”和“回滚 schema/数据”不是同一个动作。

## Week 28：v1.0 总交付与求职表达

- 学习：架构权衡、已知限制、演示叙事、STAR、系统设计问答、迭代路线。
- 代码/文档：`docs/architecture.md`、`docs/erd.md`、`docs/runbooks`、`docs/career`、`docs/demo-script.md`。
- 练习：按演示脚本从空环境启动，完成双用户成交、风险、AI 解释、故障恢复和回滚说明。
- 验收：运行本页“最终验收”，让另一位开发者只读文档即可复现。
- 产出：一个可运行、可测试、可观测、可恢复、可解释的 `exchange-lab v1.0`，以及不夸大的简历/面试材料。

## 最终验收

本机 `8080` 被占用时使用 `18080`：

```bash
API_HOST_PORT=18080 BASE_URL=http://127.0.0.1:18080 make up
make check
make integration
make backup-restore-check
BASE_URL=http://127.0.0.1:18080 make acceptance
BASE_URL=http://127.0.0.1:18080 make matching-check
BASE_URL=http://127.0.0.1:18080 make derivatives-check
BASE_URL=http://127.0.0.1:18080 make web-e2e
make fuzz-smoke
make security-check
make release-check
```

生产部署还需要你自己控制的 Linux 主机、域名、DNS 和 GitHub environment secrets。缺少这些外部资源时，本仓库交付的是可验证的生产配置与发布流程，不声称已经部署公网环境。

## 推荐官方资料

- Go Tour、Effective Go、标准库与内存模型：<https://go.dev/learn/>
- Go 数据库与取消操作：<https://go.dev/doc/database/>、<https://go.dev/doc/database/cancel-operations>
- PostgreSQL 文档：<https://www.postgresql.org/docs/current/>
- Redis 文档：<https://redis.io/docs/latest/>
- NATS JetStream 文档：<https://docs.nats.io/nats-concepts/jetstream>
- Docker Compose 文档：<https://docs.docker.com/compose/>
- React 文档：<https://react.dev/learn>
- TanStack Query 文档：<https://tanstack.com/query/latest/docs/framework/react/overview>
- OpenTelemetry Go 文档：<https://opentelemetry.io/docs/languages/go/>
- Prometheus 文档：<https://prometheus.io/docs/introduction/overview/>
- OpenAI API 文档：<https://platform.openai.com/docs/>
