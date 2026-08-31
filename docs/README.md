# Exchange Lab 文档导航

本文档是项目资料的统一入口。阅读当前实现时优先看“当前事实”；ADR 解释为什么这样设计；学习周记保留阶段演进，不作为当前运行真值。

## 推荐阅读顺序

1. [产品范围](product-scope.md)：v1.0 包含什么、不包含什么。
2. [架构](architecture.md)：进程、模块、数据所有权和一致性边界。
3. [核心 ERD](erd.md) 与 [数据字典](data-dictionary.md)：持久化模型和精度。
4. [OpenAPI](../openapi.yaml)：HTTP/WebSocket 的当前契约。
5. [已知限制](known-limitations.md)：教学简化与外部未交付项。

## 当前事实

- [产品范围](product-scope.md)
- [架构](architecture.md)
- [核心 ERD](erd.md)
- [数据字典](data-dictionary.md)
- [依赖决策登记](dependency-register.md)
- [已知限制](known-limitations.md)
- [版本号](../VERSION)

版本号以根目录 `VERSION` 为准；数据库版本以 `migrations` 和 `migrations.CurrentVersion` 为准；接口以 `openapi.yaml` 为准；运行服务和端口以 Compose 文件为准。

## 设计决策

- [ADR-001：固定小数领域值](adr/ADR-001-fixed-point-domain-values.md)
- [ADR-002：复式账本](adr/ADR-002-double-entry-ledger.md)
- [ADR-003：Transactional Outbox](adr/ADR-003-transactional-outbox.md)
- [ADR-004：JetStream 耐久交付](adr/ADR-004-jetstream-delivery.md)
- [ADR-005：确定性撮合](adr/ADR-005-deterministic-matching.md)
- [ADR-006：标记价强平](adr/ADR-006-mark-price-liquidation.md)
- [ADR-007：逐仓与全仓](adr/ADR-007-margin-modes.md)
- [ADR-008：强平、保险基金与简化 ADL](adr/ADR-008-liquidation-insurance-adl.md)
- [ADR-009：模块化单体](adr/ADR-009-modular-monolith.md)
- [ADR-010：只读 AI](adr/ADR-010-read-only-ai.md)

ADR 记录已经采用的决策。决策变化时新增 ADR 并标记替代关系，不直接把旧 ADR 改写成另一项决定。

## 领域与协议

- [行情协议与故障状态](market-data-protocol.md)
- [K 线聚合与质量](candle-quality.md)
- [Redis Key 与缓存目录](cache-catalog.md)
- [事件目录、重放与 parking](event-catalog.md)
- [撮合引擎](matching-engine.md)
- [指数价与标记价质量](price-quality.md)
- [风险与结算公式](risk-formulas.md)
- [永续合约关键序列](contract-sequences.md)
- [前端全栈调用链](frontend-call-chain.md)

## 测试与运维

- [测试策略](testing-strategy.md)
- [性能与鲁棒性基线](performance-report.md)
- [SLO 与告警](slo.md)
- [生产安全基线](security-baseline.md)
- [威胁模型](threat-model.md)
- [发布 Runbook](runbooks/release.md)
- [回滚 Runbook](runbooks/rollback.md)
- [备份恢复 Runbook](runbooks/backup-restore.md)
- [事故响应 Runbook](runbooks/incident.md)
- [强平异常 Runbook](runbooks/liquidation.md)

## 学习与演示材料

- [28 周学习与交付路线](learning/28-week-learning-map.md)
- `learning/week-01.md` 至 `learning/week-28.md`：每周代码导读、练习和当周验收快照。
- [演示脚本](demo-script.md)
- [简历项目表述](career/resume-project.md)
- [面试准备](career/interview.md)
- [求职落地计划](career/application-plan.md)

周记中的“本周”“后续”描述以对应阶段为上下文；判断当前能力时回到产品范围、架构、OpenAPI、迁移和实际代码。

## 维护规则

- 行为或接口变化必须同步更新当前事实文档和可执行契约。
- 不在 README、学习路线和专题文档中重复维护同一份长清单；由本文档链接到唯一专题。
- 本地测试产物只写入 `output-tdd/`，不把临时结果当作仓库事实。
- 无法由代码、迁移、配置或最新验证证据支持的描述，应删除或明确标为历史/计划。
