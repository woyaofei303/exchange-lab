# Week 07：K 线聚合、存储与实时推送

## 本周结果

本周完成完整链路：

```text
normalized trade
  -> durable idempotence in market_trades
  -> 1m candle atomic upsert
  -> 5m / 1h rollup
  -> PostgreSQL history
  -> NATS candle update
  -> REST + WebSocket clients
```

## 建议阅读顺序

1. `internal/candle/candle.go`：event-time 桶、OHLC、迟到修正、rollup 与 gap repair。
2. `migrations/00002_candle_pipeline.sql`：持久去重键、first/last event time 与索引。
3. `internal/platform/postgres/candle_repository.go`：成交事实和 1m Candle 的同事务写入。
4. `internal/candle/worker.go`：trade 解码、finalize、高周期重算与实时发布。
5. `cmd/candle-worker/main.go`：先订阅 NATS、再暴露健康状态的启动顺序。
6. `internal/realtime/hub.go`：有界 fan-out 和慢客户端移除。
7. `internal/httpserver/candles.go`：历史查询上限和 Candle WebSocket 生命周期。

数据策略见 `docs/candle-quality.md`，字段见 `docs/data-dictionary.md`。

## 三个时间概念

- 事件时间：交易发生时间，决定属于哪个桶以及 open/close。
- 处理时间：worker 收到消息的时间，只影响延迟，不影响 OHLC。
- watermark：当前认为已经越过的桶边界；新分钟事件到来时，之前结束的 Candle 可标 final。

迟到事件仍按事件时间修改旧 Candle。由于 `market_trades` 持久去重，同一事件重放不会重复增加 volume；高周期每次从 1m 重算，避免增量修正出现重复累计。

## 慢客户端为什么不会拖慢聚合器

聚合 worker 只发布到 NATS，不直接维护浏览器连接。API 的 hub 为每个连接提供有限缓冲；发布使用非阻塞 send，缓冲耗尽就移除该订阅。WebSocket 写还有 2 秒 deadline，因此慢网络只会关闭自己的连接。

## TDD 记录

- RED：K 线类型不存在；GREEN：事件时间 OHLC、迟到修正、overflow 和 2,000 轮性质测试通过。
- RED：rollup/gap 无规则；GREEN：1m→5m/1h、完整 final 条件与 flat zero-volume repair 通过。
- RED：慢 subscriber 可阻塞；GREEN：有界 hub 非阻塞 fan-out 并自动移除慢连接。
- RED：repository 不存在；GREEN：Testcontainers 验证 migration v2、重复成交、迟到 OHLC、finalize、history 和高周期 upsert。
- RED：历史/实时接口不存在；GREEN：严格 query、真实 WebSocket upgrade 和跨普通 request timeout 测试通过。
- RED：启动时 Core NATS 可能先发布后订阅；GREEN：candle-worker 健康后才允许 market-worker 启动。

## 动手验证

```bash
make check
make integration
API_HOST_PORT=18080 BASE_URL=http://127.0.0.1:18080 make up smoke
API_HOST_PORT=18080 BASE_URL=http://127.0.0.1:18080 make candle-check
make candle-quality
API_HOST_PORT=18080 BASE_URL=http://127.0.0.1:18080 make acceptance
make backup-restore-check
```

历史查询示例：

```bash
curl --get 'http://127.0.0.1:18080/v1/candles' \
  --data-urlencode 'symbol=BTCUSDT-PERP' \
  --data-urlencode 'interval=1m' \
  --data-urlencode 'start=2026-09-21T12:00:00Z' \
  --data-urlencode 'end=2026-09-21T13:00:00Z' \
  --data-urlencode 'limit=60'
```

## 验收结果

- [x] 1m 聚合与 5m/1h rollup。
- [x] PostgreSQL 复合主键和持久事件去重。
- [x] 重复事件不产生重复 Candle 或 volume。
- [x] 迟到、跨分钟、UTC 桶和 gap repair tests。
- [x] 历史 REST 与实时 WebSocket。
- [x] 慢连接不阻塞聚合器。
- [x] 运行态 NATS/PostgreSQL/REST/WebSocket 闭环。
- [x] OHLC、volume、唯一性与连续性数据质量检查。

## 官方资料

- [PostgreSQL INSERT / ON CONFLICT](https://www.postgresql.org/docs/current/sql-insert.html)
- [PostgreSQL date/time functions](https://www.postgresql.org/docs/current/functions-datetime.html)
- [coder/websocket concurrency](https://github.com/coder/websocket)
- [NATS Core publish/subscribe](https://docs.nats.io/nats-concepts/core-nats/pubsub)
