# ADR-003: 订单事务使用 Transactional Outbox

- 状态：已采纳
- 日期：2026-08-10
- 决策范围：订单接受、撤单请求及后续交易领域事件

## 决策

订单状态、资金冻结/释放、双向账本、余额快照、订单事件、Outbox、幂等事实和审计必须在一个 Serializable PostgreSQL 事务提交。事务中只写 `outbox_events`，不直接调用 NATS。独立 publisher 在提交后发布并更新状态，因此消息通道采用 at-least-once，所有消费者必须幂等。

Outbox payload 使用版本化 JSON，金额和数量为十进制字符串；包含稳定 event ID、aggregate ID、subject、correlation ID 与发生时间。发布成功的事实可追踪；失败使用有界重试并最终 parking，不无限循环。

## 原因与影响

直接在数据库 commit 前发消息可能出现消息已消费、业务事务却回滚；commit 后再直接发消息则可能在两步之间崩溃。Outbox 把“应发布事件”与业务事实原子化，代价是重复投递和短暂延迟，必须通过幂等 consumer 解决。

## 回滚

停用 publisher 不会丢失已提交订单，pending outbox 可在恢复后继续发布。回退 schema 前必须确认没有未发布事件并保留备份；正常发布回滚不删除 outbox。
