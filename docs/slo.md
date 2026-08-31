# SLO 与告警目标

本页是本地/参考部署的目标，不是已经获得的生产 SLA。统计窗口建议 30 天；本地验收只证明采集和告警链路可工作。

## 用户侧目标

- API availability：非故意维护窗口内，`/healthz` 成功率目标 99.9%。
- API latency：只读 API p95 < 250ms，写 API p95 < 500ms。
- Web terminal：核心合约页面可用率目标 99.5%，行情断线应在 30s 内显示明确状态。

## 业务流水线目标

- 订单 Outbox：99.9% 的事件在 5s 内 published；parked event 必须告警。
- 撮合：已接受订单事件处理 lag p95 < 2s；单交易对不允许 offset 倒退。
- 价格：可信 mark price 年龄 < 5s；质量不足时停止新强平决策并告警。
- 风险：risk scan p95 < 2s；pending liquidation job 年龄 < 10s。
- 资金费率：窗口结束后 60s 内完成；每个 settlement 的 ledger 净和必须为零。
- 账本：不平衡 transaction 数必须始终为 0，对账失败立即 P1。

## 告警优先级

- P1：账本不平衡、重复资金结算、强平重复结算、数据库数据不可读。
- P2：mark price unavailable、parked event 增长、worker lag 超限、持续 5 分钟 readiness 失败。
- P3：AI provider 错误率升高、单个 dashboard/trace exporter 不可用、容量趋势预警。

## 错误预算与处置

月度 availability 99.9% 约允许 43 分钟错误预算。消耗 50% 时冻结非可靠性变更；消耗 100% 时只允许恢复、安全和数据正确性修改。任何账本/强平不变量错误不适用“预算内可接受”，必须立即停止相关写路径。
