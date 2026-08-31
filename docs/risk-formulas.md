# 风险与结算公式

所有变量先转换为整数 atoms，乘除使用溢出检查和明确舍入。下面公式用于理解，代码真值在 `internal/fixedmath`、`position`、`margin`、`liquidation`、`funding`。

```text
notional_quote = abs(position_qty) * price / 10^quantity_scale
initial_margin = ceil(notional_quote / leverage)
maintenance_margin = ceil(notional_quote * maintenance_margin_bps / 10_000)
long_uPnL  = position_qty * (mark - entry) / 10^quantity_scale
short_uPnL = abs(position_qty) * (entry - mark) / 10^quantity_scale
margin_balance = position_margin + unrealized_pnl
margin_ratio_bps = maintenance_margin * 10_000 / margin_balance
funding_payment = -(signed_position * mark / 10^quantity_scale) * funding_rate_bps / 10_000
mark_price = index_price * (10_000 + clamped_premium_bps) / 10_000
```

保守规则：初始/维持保证金向上取整；不能精确缩放的账务结果拒绝；强平只使用 mark；价格陈旧时不得把 last 当替代风险真值。教学级 bankruptcy price/距离由 `liquidation.Evaluate` 从持仓方向、保证金余额和维持保证金确定。

逐仓边界只使用对应 `position_margins`；全仓快照聚合统一 USDT 余额、各仓位 uPnL/MM 和挂单占用，并带 `risk_version`。当前 v1 全仓是教学简化，不支持多资产折扣和组合对冲。
