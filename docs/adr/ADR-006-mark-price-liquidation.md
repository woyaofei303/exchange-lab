# ADR-006：强平只使用质量合格的标记价格

状态：Accepted

## 决策

指数价由至少两路未过期、未被异常值规则拒绝的来源聚合；标记价使用有界 premium。风险快照与强平触发只消费 mark price，last price 仅用于展示或显式配置的条件单。

## 原因与代价

last price 容易受薄深度和瞬时成交影响；mark price 提供更稳定的风险基准。代价是价格流水线更复杂，质量不足时必须停止产生新强平决策，而不是回退到不可信 last。

## 失败与回滚

来源不足或陈旧时输出 degraded/unavailable 并告警。回滚 price worker 必须保持相同质量规则和 atoms/scale 契约，不允许直接改为 last-price liquidation。
