# Runbook：强平异常

## 症状

pending job 超龄、同一风险版本疑似重复、保险余额异常、账本对账失败或强平后仓位/余额不一致。

## 处置

1. 停止 `risk-worker`，不要停止只读 API，不删除 job/event。
2. 记录 user ID、symbol、risk version、mark price ID、job ID 和 ledger transaction ID。
3. 核对 price quality、position、margin account、liquidation job/order/event、insurance entry 和 ledger 净和。
4. 若只是 worker 暂停且事实一致，恢复 worker；若事务不变量错误，保持写路径停止并前向修复。
5. 不允许手工把 job 标 completed、不允许删除幂等键或直接改余额。

## 恢复验证

```bash
docker compose start risk-worker
BASE_URL=http://127.0.0.1:18080 make derivatives-check
make ledger-check
```

恢复标准：没有新增 parked event、相同 risk version 不重复结算、账本净和为零、仓位/保证金/保险与审计能够互相解释。
