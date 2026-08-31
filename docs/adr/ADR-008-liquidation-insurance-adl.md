# ADR-008：幂等强平、保险基金与可审计简化 ADL

状态：Accepted

## 决策

强平由 `user + symbol + risk_version` 派生幂等任务。释放保证金、应用 PnL、保险覆盖、强平事件与账本在事务边界内完成。保险不足时仅生成按盈利与杠杆排序的 ADL 候选和证据。

## 原因与代价

该状态机便于故障恢复、对账和教学。当前 ADL 不执行真实生产交易所的完整自动减仓撮合，系统也不连接真实资金；这是刻意限制，不应在简历或演示中夸大。

## 失败与回滚

处理中断时任务保持可重试状态，幂等键阻止重复结算。回滚先停 risk worker，检查 pending/processing jobs、ledger reconciliation 与 insurance balance，再恢复兼容版本。
