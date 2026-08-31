# Redis Key and Cache Catalog

Redis 只保存可重建的读取模型和有明确过期时间的安全状态。订单、成交、余额、仓位、账本和最终行情历史不得以 Redis 为权威数据源。

## Global Policy

- 命名空间：`exchange-lab:v1`，版本变更时新旧 key 可并存并按 TTL 自然淘汰。
- 容量：Compose 固定 `maxmemory 128mb` 与 `maxmemory-policy noeviction`。
- 原因：缓存和登录限流、令牌撤销共用 Redis；自动淘汰安全 key 会绕过安全控制。
- 回退：市场缓存读写失败时回退权威 Reader；登录限流与 denylist 错误必须由认证层失败关闭。
- 隐私：邮箱、IP 组合身份和 token ID 在进入 key 前先规范化，再做 SHA-256；日志不记录原值或 Redis 错误内容。
- TTL：所有业务 key 必须带 TTL；无 TTL key 会使集成测试失败。

## Keys

### `exchange-lab:v1:cache:instruments:all`

- 用途：全部活跃永续合约元数据读取模型。
- 权威来源：PostgreSQL `instruments`。
- 默认 TTL：`5m`，由 `INSTRUMENT_CACHE_TTL` 配置。
- 失效：合约配置发生写入后主动删除；未命中时从 PostgreSQL 重建。
- 容量边界：只包含当前活跃合约的有限列表。

### `exchange-lab:v1:cache:instruments:symbol:<SYMBOL>`

- 用途：单合约元数据读取模型。
- 权威来源、TTL 和失效策略：同上。
- 输入约束：symbol 必须匹配大写永续合约格式，例如 `BTCUSDT-PERP`。

### `exchange-lab:v1:cache:tickers:all`

- 用途：全部合约最新 ticker 快照。
- 权威来源：规范化行情流更新最新 ticker；Redis 仅保存可重建投影，行情协议与历史数据不归 Redis 所有。
- 默认 TTL：`5s`，由 `TICKER_CACHE_TTL` 配置。
- 失效：行情更新可覆盖；过期或 Redis 清空后由权威 Reader 重建。
- 容量边界：只保留每个合约的一份最新快照，不保存历史。

### `exchange-lab:v1:cache:tickers:symbol:<SYMBOL>`

- 用途：单合约最新 ticker 快照。
- 权威来源、TTL、失效和输入约束：同 ticker 全量 key 与 instrument symbol key。

### `exchange-lab:v1:security:login:<SHA256>`

- 用途：固定窗口内的登录失败/尝试次数。
- 来源：认证入口调用原子 Lua 脚本执行 `INCR` 和首次 `PEXPIRE`。
- TTL：等于限流窗口；不存在永久 key。
- 失败策略：Redis 不可用时返回错误，认证入口拒绝继续认证，不允许失败开放。
- 容量边界：每个规范化身份在一个窗口内最多一个 key，窗口结束自动删除。

### `exchange-lab:v1:security:denylist:<SHA256>`

- 用途：记录被撤销的 token ID。
- 来源：注销、管理员撤销或安全事件。
- TTL：等于 token 剩余有效期，并受构造器 `maxTTL` 限制。
- 失败策略：写入失败不得声称撤销成功；检查失败时认证层必须失败关闭。
- 容量边界：最多为尚未自然过期的已撤销 token 数量。

## Operational Checks

```bash
docker compose exec -T redis redis-cli CONFIG GET maxmemory maxmemory-policy
docker compose exec -T redis redis-cli --scan --pattern 'exchange-lab:v1:*'
```

预期容量配置为 `134217728` 字节和 `noeviction`。发现无 TTL 业务 key、明文身份/token、非版本化 key 或权威交易数据时，应停止发布并修复。
