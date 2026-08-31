# Exchange Lab v1.0 演示脚本

建议控制在 12–15 分钟。先声明：模拟资金、教学实现、无真实托管。

## 1. 架构与启动（2 分钟）

展示 `docs/architecture.md`、`docs/erd.md` 和 Compose 服务；说明 Go 模块化单体、多 worker、PostgreSQL 真值、Redis 缓存、安全状态、JetStream 异步交付。

```bash
API_HOST_PORT=18080 BASE_URL=http://127.0.0.1:18080 make up smoke
docker compose ps
```

## 2. 双用户合约交易（4 分钟）

打开 `http://127.0.0.1:3000`，使用两个用户领取模拟资金，分别挂 maker/taker；展示订单状态、仓位、entry、mark、PnL、保证金和强平距离，刷新页面证明服务端恢复。

## 3. 风险与结算（3 分钟）

```bash
BASE_URL=http://127.0.0.1:18080 make derivatives-check
```

边执行边解释 mark price、逐仓/全仓、funding 零和、强平 job、保险基金和简化 ADL 证据。

## 4. AI 与研发 Skill（2 分钟）

展示风险助手只引用当前用户工具证据，关闭 provider 后仍确定性降级；展示 `skills/margin-risk` 或 `skills/liquidation-review` 如何约束 AI review。强调 AI 没有下单/资金写工具。

## 5. 故障与恢复（2 分钟）

```bash
BASE_URL=http://127.0.0.1:18080 make fault-injection
```

展示 NATS 停止时 readiness 失败、恢复后自动就绪；说明 Outbox、dedupe、parking、backup 和 immutable rollback。

## 6. 收尾（1 分钟）

展示 `docs/known-limitations.md` 和 `docs/README.md`，明确已经实现、教学简化和仍需外部环境的部分。
