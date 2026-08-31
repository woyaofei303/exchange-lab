# Runbook：回滚

## 原则

应用镜像回滚是可恢复操作；schema down 和数据恢复可能破坏数据，默认不执行。先判断前一版本是否能读取当前 schema，优先回退二进制或前向修复。

## 流程

1. 停止继续发布并保存当前/前一 tag、故障时间和证据。
2. 若新 migration 向后兼容，使用 previous commit-SHA image 重启，等待 health/readiness 并跑 smoke。
3. 若 schema 不兼容，保持写入停止，使用已验证方案前向修复；只有经审批且确认恢复点后才恢复数据库。
4. 回滚后核对 schema version、账本、Outbox/consumer lag、parked event、仓位、强平和资金费率窗口。

```bash
./deploy/rollback.sh
```

没有 `.release-previous`、previous image 不存在、备份未验证或账务不变量异常时脚本/人工流程都必须停止。
