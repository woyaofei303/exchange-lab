# Runbook：通用事故响应

## 触发与停止条件

账本不平衡、重复结算或疑似越权按 P1；mark price 不可用、parked event 增长或关键 worker lag 按 P2。不要先删除数据、重置 consumer 或重启全部服务。

## 流程

1. 记录开始时间、版本 `IMAGE_TAG`、请求/trace/event ID、影响用户和最后正常时间。
2. 用 `/healthz`、`/readyz`、Prometheus/Grafana 和容器状态区分进程、依赖与业务不变量。
3. 保存有界日志；禁止复制 password、cookie、DSN、API key 或完整用户 payload。
4. 数据正确性可疑时停止对应 writer/worker，保留数据库、Outbox、JetStream、dedupe 和 parked facts。
5. 选择缓解：回退不可变镜像、恢复依赖、隔离坏输入或前向修复。不可逆数据操作需另行审批和备份。
6. 恢复后运行对应 acceptance、ledger reconciliation、worker lag 与新 parked event 检查。
7. 24 小时内写 timeline、root cause、检测缺口、修复、owner 和截止时间。

## 快速证据

```bash
curl --silent http://127.0.0.1:18080/healthz
curl --silent http://127.0.0.1:18080/readyz
docker compose ps
docker compose logs --since 15m --tail 500 api event-worker price-worker risk-worker funding-worker
```
