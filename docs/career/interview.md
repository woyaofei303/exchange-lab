# 面试准备：Exchange Lab

## 60 秒介绍

我用 28 周路线做了一个模拟 USDT 本位永续合约系统，重点不是堆页面，而是打通精确数值、复式账本、订单 Outbox、JetStream 幂等消费、确定性撮合、仓位/保证金/强平/资金费率、React 终端、只读 AI 工具和发布恢复。系统使用 Go 模块化单体加独立 worker，PostgreSQL 是权威事实，Redis 只承载可重建缓存与短期安全状态。它不连接真实资金，ADL 和容量也是明确的教学简化。

## 高频追问

1. 为什么不用 `float64`？说明二进制表示误差、atoms/scale、decimal string 和溢出测试。
2. 下单成功但消息失败怎么办？说明同事务 Outbox、publisher ACK、at-least-once 和 consumer dedupe。
3. 重复事件怎么办？说明稳定 event ID、`processed_events` 与副作用同事务。
4. 为什么 mark price 强平？说明 last 可操纵、多源 index、stale/outlier 和 premium clamp。
5. funding 如何零和？说明 long/short transfer、舍入与 system offset、ledger reconciliation。
6. 强平中途崩溃怎么办？说明 risk version 幂等 job、事务、状态机、保险/账本证据和 runbook。
7. 为什么不是微服务？说明一个人项目的复杂度预算、模块边界和有证据后再拆分。
8. AI 为什么只读？说明非确定性、证据 schema、权限隔离、fallback、eval 和审计。
9. 如何证明可发布？说明 CI、integration/E2E、backup restore、security/image scan、immutable tag、health gate 和 rollback。
10. 最大限制是什么？诚实回答单交易对/单节点/模拟行情/简化 ADL/无公网生产环境。

准备时用具体文件、测试和失败案例回答，不背抽象术语。
