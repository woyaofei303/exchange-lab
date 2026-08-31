# 简历项目表述（可按原模板字段粘贴）

项目名称：Exchange Lab — Go 全栈永续合约交易系统实验室

项目简介：通过代码学习并独立实现的模拟 USDT 本位永续合约系统，不接入真实资金，覆盖行情、K 线、订单、撮合、仓位、保证金、资金费率、强平、保险基金、React 终端、只读 AI 风险助手、可观测性与发布恢复。

所用技术：Go、PostgreSQL、Redis、NATS JetStream、React、TypeScript、TanStack Query、KLineChart、Docker Compose、OpenTelemetry、Prometheus、Grafana、Playwright、GitHub Actions、OpenAI structured output、MCP、项目自定义 Skills。

技术难点：使用整数 atoms + scale 保证价格、数量、保证金与 PnL 精度；以 Serializable 事务、复式账本、Transactional Outbox 和消费幂等保证资金/事件一致性；实现基于 mark price 的逐仓/全仓风险、资金费率零和结算与幂等强平状态机。

工作内容：以测试驱动方式完成 16 个数据库迁移和模块化 Go 服务；实现价格时间优先撮合、双用户合约终端和真实依赖/E2E 验收；为 AI/MCP 设计只读、证据绑定和审计边界，并创建项目 Skills 辅助需求开发与代码审查；建立 dashboard、告警、备份恢复、安全扫描、不可变镜像发布与回滚文档。

项目业绩：形成可本地复现的端到端合约交易闭环和 28 周学习文档；核心领域具有 unit/property/fuzz/race/integration/E2E 覆盖，迁移 v1–v16、50 表备份恢复、资金费率/账本零和及故障恢复均有自动化验收证据。

注意：这是个人学习/作品项目。不要写成真实交易所生产规模、真实资金系统、部门负责人或已公网部署。
