# ADR-005：单分区确定性价格时间撮合

状态：Accepted

## 决策

v1 每个撮合消费者单消息 in-flight，以价格优先、同价时间优先和 maker price 生成确定性成交；订单簿快照、offset、成交、订单状态、Outbox 与 dedupe 同事务提交。

## 原因与代价

确定性和可回放性优先于水平扩展，适合学习系统与单交易对演示。代价是单 consumer 吞吐上限、全量 symbol snapshot 成本，以及无法代表生产交易所的低延迟内核。

## 失败与回滚

事务失败不 ACK；重投命中 event ID 幂等。回滚先停 matching consumer，核对 snapshot 与 offset，再用兼容二进制继续；禁止只改 offset 而不核对 book/trade。
