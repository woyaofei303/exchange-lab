# 第 11 周：下单、风控、冻结与 Transactional Outbox

## 学习重点

- 限价单过滤：symbol、side、tick、step、最小名义价值和杠杆。
- 下单预留：`ceil(notional / leverage)`，中间计算不使用浮点数。
- Serializable 事务、advisory idempotency lock、余额行锁与锁顺序。
- Transactional Outbox 为什么解决 DB 与消息的双写窗口。
- 所有权、重复请求、撤单状态机和资金精确释放。

## 代码阅读顺序

1. `internal/order/order.go`：订单构造、初始保证金和状态转换。
2. `internal/order/service.go`：输入规范化、合约过滤与请求 hash。
3. `migrations/00005_orders_outbox.sql`：订单、事件、幂等和 Outbox 约束。
4. `internal/platform/postgres/order_repository.go`：沿事务顺序核对每个失败点的回滚范围。
5. `internal/httpserver/orders.go`：session 所有权、严格 JSON 和 decimal string。
6. `scripts/ordercheck.sh`：真实重复下单/撤单、余额与 Outbox 证据。

## 动手练习

```bash
go test ./internal/order ./internal/httpserver
make integration
API_HOST_PORT=18080 BASE_URL=http://127.0.0.1:18080 make order-check
```

临时让 Outbox 插入失败，验证订单与冻结余额都没有留下；再并发发送同一幂等键，确认只生成一个订单和一组账本 entry。

## 完成标准

- 下单成功时订单、冻结、账本、Outbox、幂等与审计一起存在。
- 任一步失败时全部回滚。
- 同键同参数返回原结果；同键不同参数冲突。
- 撤单只释放一次且不能释放其他用户订单。
