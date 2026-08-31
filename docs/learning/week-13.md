# 第 13 周：订单簿与撮合引擎

## 学习重点

- bid/ask、price level、spread、marketable limit。
- 价格优先、同价时间优先和 maker price。
- partial fill、remaining、cancel、确定性 replay。
- 纯领域核心与事件/数据库边界分离。
- fuzz/property、race 和 benchmark 如何验证不变量而非只测示例。

## 代码阅读顺序

1. `internal/matching/engine.go`：从 `Add` 的 best opposite 循环读起。
2. `internal/matching/engine_test.go`：手工测试向量、确定性回放、fuzz 与 benchmark。
3. `migrations/00007_matching.sql`：book snapshot、offset、trade 和 matching event。
4. `internal/platform/postgres/matching_repository.go`：事件去重、offset 锁、重建、应用、快照与 trade Outbox 原子提交。
5. `cmd/event-worker/main.go` 的 `handleMatchingMessage`：ACK/NAK/parking 边界。

## 动手练习

```bash
go test ./internal/matching
go test -race ./internal/matching
go test -fuzz=FuzzEngineNeverOverfills -fuzztime=10s ./internal/matching
go test -run '^$' -bench BenchmarkEngineMatch -benchmem ./internal/matching
API_HOST_PORT=18080 BASE_URL=http://127.0.0.1:18080 make matching-check
```

增加三档卖价和同价三笔订单，手算交易顺序；随后调换输入 sequence，确认只有合法的时间顺序改变结果。

## 完成标准

- 固定输入和快照得到固定 trade/book 输出。
- maker price、价格优先、FIFO、部分成交和撤单测试通过。
- fuzz 证明任何成交都为正且不超过双方原数量。
- 真实双用户订单经 JetStream 撮合，trade、订单状态和 book 可核对。
