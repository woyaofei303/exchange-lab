# Exchange Lab 数据字典

本文件记录 PostgreSQL 权威数据模型。所有时间字段使用 `TIMESTAMPTZ` 并按 UTC 写入；价格、数量和金额使用整数 atoms 加显式 scale，不使用数据库浮点类型。

## schema_migrations

由 goose 管理，不由业务代码直接写入。

- `id`：迁移记录主键。
- `version_id`：迁移版本。
- `is_applied`：该版本当前是否已应用。
- `tstamp`：迁移记录时间。

API readiness 要求最新已应用版本不低于二进制声明的 `migrations.CurrentVersion`。

## users

用户表支持注册、登录、Argon2id 密码哈希与 RBAC。

- `id BIGINT IDENTITY PRIMARY KEY`：用户内部 ID。
- `email TEXT NOT NULL`：登录邮箱；`lower(email)` 唯一索引提供大小写不敏感唯一性。
- `password_hash TEXT NOT NULL`：Argon2id PHC 风格编码；禁止原始密码、session token 或可逆密文。
- `role TEXT NOT NULL`：`user` 或 `admin`，默认 `user`；所有授权使用服务端读取值。
- `status TEXT NOT NULL`：`active` 或 `disabled`。
- `created_at TIMESTAMPTZ NOT NULL`：创建时间。
- `updated_at TIMESTAMPTZ NOT NULL`：最后更新时间。

## user_sessions

- `token_hash BYTEA PRIMARY KEY`：32 字节 SHA-256；原始 opaque token 永不落库。
- `user_id BIGINT NOT NULL`：用户外键，删除用户时级联删除 session。
- `created_at / expires_at / revoked_at TIMESTAMPTZ`：UTC 生命周期；refresh 在一个 Serializable 事务中撤销旧行并插入新行。
- 活跃索引只包含 `revoked_at IS NULL`；过期判定仍由 repository/service 执行。

## instruments

永续合约过滤器的 PostgreSQL 真值；`/v1/instruments` 从此表读取。

- `symbol TEXT PRIMARY KEY`：规范合约代码，例如 `BTCUSDT-PERP`。
- `base_asset TEXT NOT NULL`：标的资产，例如 `BTC`。
- `quote_asset TEXT NOT NULL`：保证金和结算资产，例如 `USDT`。
- `base_scale SMALLINT NOT NULL`：标的资产账务精度。
- `quote_scale SMALLINT NOT NULL`：报价资产账务精度。
- `price_scale SMALLINT NOT NULL`：价格精度。
- `quantity_scale SMALLINT NOT NULL`：数量精度。
- `price_step_atoms BIGINT NOT NULL`：最小价格步长的整数 atoms。
- `quantity_step_atoms BIGINT NOT NULL`：最小数量步长的整数 atoms。
- `min_notional_atoms BIGINT NOT NULL`：按 `quote_scale` 表示的最小名义价值 atoms。
- `status TEXT NOT NULL`：`active` 或 `disabled`。
- `created_at TIMESTAMPTZ NOT NULL`：创建时间。
- `updated_at TIMESTAMPTZ NOT NULL`：最后更新时间。

关键约束：base 与 quote 不同；symbol 必须等于 `base_asset || quote_asset || '-PERP'`；`price_scale + quantity_scale <= quote_scale`；所有步长和最小名义价值必须为正数。

默认数据：

```text
BTCUSDT-PERP
base_scale=8
quote_scale=8
price_scale=2
quantity_scale=3
price_step_atoms=10       -> 0.10
quantity_step_atoms=1     -> 0.001
min_notional_atoms=500000000 -> 5.00000000 USDT
```

## candles

Migration 1 建立基础约束，migration 2 增加幂等成交写入、聚合和查询所需字段。

- `symbol TEXT`：外键到 `instruments.symbol`。
- `interval_seconds INTEGER`：K 线周期秒数。
- `open_time TIMESTAMPTZ`：UTC 桶起点。
- `price_scale SMALLINT`：OHLC atoms 的价格精度。
- `quantity_scale SMALLINT`：成交量 atoms 的数量精度。
- `open_atoms/high_atoms/low_atoms/close_atoms BIGINT`：固定精度 OHLC。
- `volume_atoms BIGINT`：非负成交量。
- `trade_count BIGINT`：非负成交笔数。
- `is_final BOOLEAN`：该时间桶是否已封闭。
- `first_trade_time/last_trade_time TIMESTAMPTZ`：按事件时间确定 open/close；迟到事件可正确修正。
- `first_sequence/last_sequence BIGINT`：同一事件时间时用于稳定选择 first/last。
- `updated_at TIMESTAMPTZ`：最后聚合时间。

主键为 `(symbol, interval_seconds, open_time)`，保证同一合约、周期和时间桶只有一条记录。约束保证 high 不低于 open/close/low，low 不高于 open/close/high。

1m Candle 与 `market_trades` 在同一事务更新。5m/1h Candle 从 1m 数据重新 rollup 后执行完整值 upsert，使迟到修正和 gap repair 可重复执行。

## market_trades

规范化 trade 的最小幂等事实表，不保存 WebSocket 原始 frame。

- `stream_id + trade_id PRIMARY KEY`：跨进程重启的业务去重键。
- `stream_id + sequence UNIQUE`：防止同一 stream sequence 被不同 trade ID 重放。
- `symbol`：合约外键。
- `sequence`：规范化行情全局序列。
- `event_time`：成交事件时间，决定 K 线开收顺序和时间桶。
- `price_scale/quantity_scale`：整数值精度。
- `price_atoms/quantity_atoms`：正整数成交价格与数量。
- `received_at`：系统接收并持久化时间，仅用于诊断，不参与 OHLC。

`(symbol, event_time)` 索引用于 gap repair、回放和数据质量排查。重复插入使用 `ON CONFLICT DO NOTHING`，只有首次插入的事件能增加 Candle volume/trade_count。

## audit_logs

审计表只追加；身份、授权、账本和交易敏感操作写入结构化审计事实。

- `id BIGINT IDENTITY PRIMARY KEY`：审计记录 ID。
- `actor_user_id BIGINT NULL`：可为空的用户外键；删除用户后保留审计事实并置空该引用。
- `action TEXT NOT NULL`：动作代码。
- `resource_type TEXT NOT NULL`：资源类型。
- `resource_id TEXT NOT NULL`：资源业务标识。
- `metadata JSONB NOT NULL`：对象结构的非敏感扩展信息。
- `occurred_at TIMESTAMPTZ NOT NULL`：发生时间。

`(resource_type, resource_id, occurred_at DESC)` 索引用于按资源倒序追踪审计历史。metadata 禁止保存密码、原始 token、完整 DSN 和私钥。

## ledger_accounts

用户拥有 `available` / `locked` 账户；平台拥有 `system_faucet`、`fees`、`insurance` 账户。`owner_user_id + asset + account_type` 使用 `NULLS NOT DISTINCT` 唯一约束，防止同类账户重复。

## balances

认证用户按资产维护的物化余额快照。`available_atoms` 与 `locked_atoms` 均为非负 `BIGINT`；`version` 每次写入递增。写路径必须在同一事务内 `FOR UPDATE` 锁行并同步追加账本，不允许把此表当作没有历史的普通 CRUD 表。

## ledger_transactions / ledger_entries

- `ledger_transactions`：不可变业务事务头；`idempotency_key` 全局唯一。
- `ledger_entries`：事务借贷行，`amount_atoms` 为有符号非零 `BIGINT`；每个业务事务至少两行、同一资产合计必须为零。
- 普通业务代码不得更新或删除已经提交的事务和 entry；纠错必须追加反向事务。

## faucet_requests

模拟入金的业务幂等事实，记录用户、USDT 金额、账本事务和键。相同键、用户和金额返回原结果；同键不同参数返回冲突。水龙头、账本、余额与审计必须在一个 Serializable 事务提交。

## orders / order_events

`orders` 是用户订单快照，保存精确的价格/数量 atoms、各自 scale、剩余数量、杠杆和预留 USDT 保证金。`user_id + client_order_id` 唯一。`order_events` 是按订单追加的版本化生命周期事实，普通业务不覆盖历史事件。

## idempotency_keys

按 `user_id + operation + idempotency_key` 唯一，保存规范请求的 SHA-256 与最终资源 ID。完全相同的重放返回原资源；不同请求 hash 使用相同键时拒绝。原始密码、token 和请求 body 不进入此表。

## outbox_events

与订单/账本同事务写入的待发布事件。状态为 `pending / publishing / published / parked`，payload 是版本化 JSON object，金额值为字符串。独立 publisher 负责 at-least-once 发布；Outbox 不是消费者幂等的替代品。

## processed_events / event_receipts

`processed_events` 的主键是 `(consumer_name,event_id)`；它与消费者业务副作用 `event_receipts` 在一个 Serializable 事务提交。重复投递命中主键后不再执行副作用，但仍可安全 ACK。

## parked_events

保存不兼容 envelope 或达到最大投递次数的事件证据：consumer、event ID、subject、固定原因码、投递次数和有界 JSON payload。不得存原始异常、凭据或无限 payload。恢复必须按 Runbook 做受控重放。

## matching_book_orders / matching_offsets

`matching_book_orders` 是可从已处理订单事件重建的单交易对订单簿快照，保存 maker 的原始数量、剩余数量、价格和 JetStream 接受顺序。`matching_offsets` 锁定 `matching-v1` 的最后 stream/trade sequence，保证事件顺序和确定性 trade 编号。

## contract_trades / matching_events

`contract_trades` 是不可变成交事实，价格/数量使用 atoms + instrument scale，包含 maker/taker 订单与用户、方向和全局 matching sequence。`matching_events` 记录每个 source event 导致的订单 open/matched/partial/filled/canceled 状态。成交、订单状态、book、offset、trade Outbox、dedupe 和 receipt 同事务提交。

## contracts / leverage_tiers

`contracts` 保存永续合约风险参数和启用状态；`leverage_tiers` 按名义价值区间给出最大杠杆与 maintenance margin BPS。更新 tier 必须版本化并触发风险评审，不允许把前端值当真值。

## price_sources / index_prices / mark_prices

`price_sources` 是带来源时间和接收时间的整数价格输入；`index_prices` 保存纳入/排除来源证据；`mark_prices` 保存 index、premium BPS、最终 mark 和质量时间。历史只追加，风险读取最新合格 mark。

## positions / position_events / position_snapshots / trade_settlements

`positions` 是每用户、合约的单向仓位快照：有符号 size、entry、累计 realized PnL、fee 和 version。`position_events` 与 `trade_settlements` 按 trade/event 幂等记录应用事实；`position_snapshots` 保存可诊断历史。HTTP 小数由 atoms + instrument scale 格式化。

## margin_accounts / leverage_settings / position_margins / risk_limits / margin_adjustments

`margin_accounts` 保存用户的 isolated/cross 模式和单调递增 risk version；`leverage_settings` 按合约保存杠杆；`position_margins` 保存逐仓分配；`risk_limits` 保存维护保证金规则；`margin_adjustments` 用幂等键记录余额与逐仓保证金转移。资金变化必须与余额/账本在同一事务。

## cross_risk_snapshots

全仓风险的只追加证据，保存 wallet、order margin、position margin、unrealized PnL、maintenance、ratio、版本与时间。它是诊断快照，不替代 `balances`、`positions` 和最新 mark 的权威事实。

## liquidation_jobs / liquidation_orders / liquidation_events

`liquidation_jobs` 以 user、symbol、risk version 派生幂等键，记录触发 mark、bankruptcy price 和状态；`liquidation_orders` 保存 reduce-only 执行事实；`liquidation_events` 保存有界 JSON 证据。不得手工覆盖完成状态或删除事件来重试。

## premium_indexes / funding_rates / funding_settlements

每个 funding window 的 premium、预测/最终 BPS 和 settlement 状态只记录一次。`funding_settlements` 按用户保存有符号 position 和 payment atoms；结算 ledger 加总必须为零。模拟单边 exposure 通过 system/insurance offset 平衡，不能写零金额 ledger entry。

## insurance_fund_entries / adl_candidates / adl_events

保险基金使用只追加 entry 和 `balance_after_atoms` 证据；bankruptcy loss 只能通过强平事务借记。保险不足时 `adl_candidates` 保存盈利、杠杆、rank 与 evidence；v1 ADL 只做教学排序与审计，不代表完整执行。

## conditional_orders

用户 TP/SL 条件单保存 mark/last trigger source、gte/lte direction、精确 trigger price/quantity、reduce-only 状态和幂等键。触发只产生普通订单/Outbox 事实，重复扫描不能重复下单。

## ai_runs

保存用户、合约、model、结果状态、token usage、允许的 evidence IDs 和时间。禁止保存 API key、session、完整 prompt 或其他用户数据。AI run 不拥有风险计算真值。

## tool_audit_logs

MCP/AI 只读工具审计：user、tool name、固定 outcome、request/correlation ID、脱敏 metadata 和时间。工具失败不写交易事实；审计禁止包含 password、cookie、DSN、API key 或完整模型 payload。
