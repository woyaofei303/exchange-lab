# v1.0 已知限制

- 仅模拟 USDT 本位永续合约和本地资金，不连接真实充值、提现、托管、KYC、法币或外部清算。
- 主要演示单交易对、单分区撮合；没有生产交易所级分片、低延迟日志、灾备集群或多地域一致性。
- 行情来源是确定性 simulator；指数源、深度和流动性不代表真实市场。
- 风险 tier、手续费、funding 和 bankruptcy/ADL 是教学模型；ADL 只生成可审计排序证据，不执行完整生产自动减仓。
- PostgreSQL/Redis/NATS 的本地 Compose 为单节点；生产 HA、PITR、加密 KMS 和跨故障域备份需部署环境提供。
- AI 助手是只读解释层，可能不可用或降级，不是财务建议，不参与权威风险计算或交易写入。
- 公网 staging 需要用户提供 Linux VM、域名、DNS、GitHub environment secrets 和 on-call；仓库未声称这些外部资源已部署。
- 本地 benchmark/load 只提供回归基线，不能外推生产容量或可用性承诺。

后续可选方向：真实行情 adapter、撮合分区与 event log、portfolio/risk tier 扩展、Kubernetes/managed data services、多区域灾备、完整 ADL 执行、合规与安全审计。每项都必须先定义数据所有权、故障模式和回滚。
