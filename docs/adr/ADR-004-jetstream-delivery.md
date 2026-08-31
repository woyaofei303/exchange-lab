# ADR-004：JetStream 负责耐久事件交付

状态：Accepted

## 决策

订单与成交事件使用 NATS JetStream 的 file-backed stream、durable explicit-ack consumer 和有界 redelivery。PostgreSQL Transactional Outbox 是发布前权威事实；`processed_events` 是消费幂等真值。JetStream 不替代订单、成交或账本数据库。

## 原因与代价

该方案保留低运维成本和清晰的 at-least-once 语义，可演练 ACK 丢失、重放与 parking。代价是每个消费者必须实现幂等，且 stream retention 不能当永久审计。

## 失败与回滚

NATS 不可用时 Outbox 积压；消费失败有界重投后 parking。回滚先停 publisher/consumer，保留 stream volume、Outbox 和 dedupe，再回退应用。禁止通过删除 dedupe 表强迫重放。
