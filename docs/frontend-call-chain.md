# 前端全栈调用链

## 历史 K 线

```text
React KLinePanel
  -> GET /v1/candles (symbol, interval, UTC start/end, bounded limit)
  -> Go httpserver validates identity and range
  -> PostgreSQL CandleRepository returns integer atoms + scale
  -> Go formats exact decimal strings
  -> frontend contract decoder validates version/shape/time/decimal
  -> KLineChart adapter converts strings to Number for drawing only
```

## 实时 K 线

```text
market-simulator -> market-worker -> NATS market.*.trade
  -> candle-worker transactionally deduplicates trade and upserts candle
  -> NATS candles.<symbol>.<interval>
  -> API realtime.Hub -> /ws/v1/candles
  -> CandleStream validates frame and pushes one bucket to KLineChart
```

历史与实时以 `(symbol, interval, open_time)` 作为同一身份。相同 `open_time` 的实时更新覆盖当前 bar，不追加重复 bar。切换交易对或周期时，KLineChart 触发 unsubscribe，旧 WebSocket 被关闭；新的 REST 与 WebSocket 使用新的完整身份。

## 状态与故障

- `connecting`：首次建立实时连接。
- `live`：WebSocket 已打开；不等价于数据一定新鲜，ticker 的 `updated_at` 仍显示来源时间。
- `reconnecting`：连接关闭后以 250ms 起步、最大 5s 的退避重连。
- `offline`：组件卸载或主动切换后旧订阅已关闭。
- 历史 REST 失败时页面显示明确错误，不用空数组伪装成功。
- 非法 JSON、未知版本、数值型金额或错误时间在浏览器边界被丢弃。

## 精度边界

REST/WebSocket 的 price、quantity、volume、money、margin、PnL 必须是十进制字符串。只有 KLineChart 要求数值时，`chartCandle` 才执行 `Number(...)`；该近似值不得用于下单、风控、保证金或账本。模拟深度用 `BigInt` 对 atoms 做展示计算。

## 回滚

Web 是只读消费者，可停止 `web` 服务并继续直接使用 Go API，不影响 PostgreSQL、Redis 或 NATS 数据。Go `web-gateway` 只负责静态资源、同源 REST/WS 代理与安全头。替换图表库时保留 frontend contract decoder、REST/WS 同一身份、unsubscribe、退避、错误态和 E2E 断线恢复测试。

## 合约交易终端

```text
Login cookie -> GET balances/orders/positions/risk
  -> TradingPanel keeps server state in TanStack Query
  -> POST order/margin/mode/leverage uses same-origin cookie + strict body
  -> API transaction commits PostgreSQL fact + Outbox
  -> JetStream matching -> trade -> position consumer
  -> query invalidation/refetch renders position and liquidation distance
```

浏览器不计算权威保证金、PnL、强平价格或资金费率。它只验证响应 shape 并显示后端 decimal string；`liquidation_distance_bps` 是后端风险字段。双用户 Playwright 使用两个独立 cookie context，通过真实 API 挂单成交，刷新后从 PostgreSQL 恢复状态，再 reduce-only 关闭。

## AI 调用链

`/v1/ai/contract-risk-summary` 先鉴权，再由只读工具按当前 user ID 获取 position/risk/funding evidence。模型只接收裁剪后的证据并输出结构化 summary；证据缺失、schema 失败或 provider 不可用返回脱敏失败/降级，不影响交易查询与写入。前端不得把 AI 文本转成下单参数。
