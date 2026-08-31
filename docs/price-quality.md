# 指数价与标记价质量

price-worker 从规范 ticker 生成三个独立命名的模拟现货源，先过滤超过 10 秒的陈旧源，再用中位数基线排除偏差超过 100 bps 的源，至少保留 3 个源，最后按整数权重求价。结果保存 included/excluded source、最老来源时间和 freshness。

标记价为 `index + bounded premium`，premium 限制在 ±100 bps。API `/v1/prices` 暴露 index、mark、premium、source 证据和时间。风险快照、uPnL、条件单 mark 触发和强平均从 `mark_prices` 读取；`contractprice.Snapshot` 类型故意没有 last price 字段。

故障策略：不足 3 个新鲜来源时拒绝产生新标记价；已有快照随时间变陈旧。v1 的三个源是确定性模拟，不代表真实交易所源独立性；接真实只读源时必须保留来源级 timeout、异常过滤、权重审计和回退演练。
