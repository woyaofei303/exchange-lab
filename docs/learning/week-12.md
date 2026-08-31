# 第 12 周：JetStream、事件流与幂等消费者

## 学习重点

- Stream、subject、file storage、retention 和 publish acknowledgement。
- Durable pull consumer、显式 ACK、AckWait、MaxDeliver、backoff 与 MaxAckPending。
- at-least-once 的含义：网络超时或 ACK 丢失都可能产生重复投递。
- Outbox lease、`FOR UPDATE SKIP LOCKED`、幂等副作用事务、parking 与重放。

当前官方 NATS 文档说明：`Fetch` 是有界批次，`Consume` 是长运行 pull 流；处理成功后才 ACK，空批次是正常状态。实现采用长运行 `Consume`，但领取、消息、重试和并发全部有界。

## 代码阅读顺序

1. `internal/eventing/eventing.go`：envelope 与 publisher 状态机。
2. `internal/eventing/jetstream.go`：ORDERS/TRADES stream 和 durable consumer 配置。
3. `internal/platform/postgres/event_repository.go`：Outbox lease、发布状态、幂等 receipt 和 parking。
4. `cmd/event-worker/main.go`：publisher/consumer 生命周期、ACK/NAK/TERM 与健康检查。
5. `scripts/eventcheck.sh`：真实 Outbox → JetStream → consumer，以及主动重复投递。

## 动手练习

```bash
go test ./internal/eventing
make integration
API_HOST_PORT=18080 BASE_URL=http://127.0.0.1:18080 make event-check
docker compose restart nats event-worker
```

在 `RecordReceipt` commit 前让进程失败，观察 redelivery；恢复后确认 `processed_events` 和 `event_receipts` 仍只有一条。

## 完成标准

- 订单 Outbox 最终变为 published，durable consumer receipt 可查询。
- 同 event ID 的两次真实投递只产生一次副作用。
- consumer crash/NATS 重启后继续，未 ACK 消息重投。
- 非法或耗尽事件进入 parking，重试次数和队列都有上限。
