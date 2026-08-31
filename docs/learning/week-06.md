# Week 06：行情模拟、WebSocket 与有限重连

## 本周结果

本周完成了一条可运行的行情链路：

```text
market-simulator WebSocket
  -> market-worker 严格解码与序列状态机
  -> NATS Core 规范化事件
  -> Redis 最新 ticker
  -> API /v1/tickers
```

默认只使用本地 simulator，因此断网、重复、乱序、gap、非法 JSON 和慢下游都能在测试中确定性复现，不受第三方交易所稳定性影响。

## 建议阅读顺序

1. `internal/marketdata/protocol.go`：事件 envelope、四类 payload、严格 JSON 与十进制定点校验。
2. `internal/marketdata/simulator.go`：仅用整数 atoms 生成 snapshot、ticker、trade、delta。
3. `internal/marketdata/sequencer.go`：snapshot 基线、去重、乱序和 gap 状态机。
4. `internal/marketdata/worker.go`：有限重连、指数退避、heartbeat、慢下游 timeout 与交付边界。
5. `internal/platform/websocketfeed`：`coder/websocket` 的真实 Dial/Read/Ping/Close 适配器和 simulator handler。
6. `internal/platform/natspublisher`：NATS Core Publish + 有 deadline 的 Flush。
7. `cmd/market-simulator` 与 `cmd/market-worker`：进程装配和 Compose 配置。
8. `skills/websocket-reliability/`：以后实现或审查流式连接时复用的项目 Skill。

协议与异常状态的完整定义见 `docs/market-data-protocol.md`。

## 关键理解

### Ping 为什么必须与 Read 并发

WebSocket 的 Pong 是控制帧，需要 reader 持续消费连接。停止读取后再同步调用 Ping，可能一直等不到 Pong。当前 worker 始终只有一个读取循环，heartbeat 在另一个 goroutine 中 Ping；任一方失败都会取消本次 session，并在退出前回收 heartbeat goroutine。

### 为什么 gap 不能直接跳过

订单簿 delta 缺一条后，继续应用后续 delta 会得到“看起来合理但实际错误”的簿。发现 `received > last + 1` 时，worker 只记录期望/收到的序列号，关闭本次连接，通过新 stream 的 snapshot 重建状态。

### 为什么 NATS 和 Redis 失败策略不同

NATS 是规范化事件的交付边界；发布失败意味着下游确实漏事件，因此断开 session。Redis 只保留可重建的最新 ticker；写入失败可以降级，后续事件或权威 Reader 会再次填充。

## TDD 记录

- RED：`internal/marketdata` 不存在；GREEN：协议、sequencer、simulator、worker 的单元与 fuzz tests 通过。
- RED：非法 JSON、unknown field、数值型价格、payload 冲突未受限；GREEN：严格 decoder 在状态变更前拒绝。
- RED：重复、乱序和 gap 没有确定语义；GREEN：独立状态机固定四种 disposition。
- RED：慢 publisher 可永久阻塞；GREEN：每次 NATS/Redis 操作有 deadline，publisher timeout 触发有限重连。
- RED：真实 WebSocket Ping 测试失败；诊断发现测试未并发读取；GREEN：测试和生产均采用 single reader + concurrent Ping。
- RED：Compose 对 4 个进程重复构建同一镜像；GREEN：`make up` 单次构建共享镜像。
- RED：worker 只更新 symbol ticker，API 全量 key 仍旧；GREEN：同时更新 `:all` 与 symbol key，并增加 Redis 集成断言。

## 动手验证

运行单元、race、构建与 Compose 合约：

```bash
make check
```

运行 PostgreSQL 与 Redis Testcontainers：

```bash
make integration
```

启动完整环境（本机 8080 已占用时使用 18080）：

```bash
API_HOST_PORT=18080 BASE_URL=http://127.0.0.1:18080 make up smoke
make market-check
```

查看实时 ticker 和进程：

```bash
curl --silent http://127.0.0.1:18080/v1/tickers
docker compose ps
docker compose logs --tail=100 market-worker market-simulator
```

执行 Redis、NATS、PostgreSQL 故障恢复并再次检查行情链路：

```bash
API_HOST_PORT=18080 BASE_URL=http://127.0.0.1:18080 make acceptance
make market-check
```

## 验收结果

- [x] simulator 生成 ticker、trade、orderbook snapshot/delta。
- [x] decoder 拒绝非法 JSON、未知字段、数字小数、错误 payload 和二进制帧。
- [x] duplicate/out-of-order 不重复发布；gap 报告并重连。
- [x] heartbeat、发布、缓存、退避、消息大小和重连次数全部有界。
- [x] `go test -race ./...` 通过。
- [x] 真实 WebSocket → NATS + Redis → API 链路通过。
- [x] Redis/NATS/PostgreSQL 故障恢复后行情链路继续工作。
- [x] `websocket-reliability` Skill 通过官方结构校验。

## 官方资料

- [coder/websocket repository and documentation](https://github.com/coder/websocket)
- [NATS Go client](https://github.com/nats-io/nats.go)
- [NATS Core publish/subscribe](https://docs.nats.io/nats-concepts/core-nats/pubsub)
- [Go fuzzing](https://go.dev/doc/security/fuzz/)
