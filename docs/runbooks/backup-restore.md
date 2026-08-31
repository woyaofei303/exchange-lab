# Runbook：备份与恢复

## 备份

`scripts/backup.sh` 使用 PostgreSQL custom-format dump，输出到 `output-tdd/backups`。生产环境必须把备份加密复制到故障域之外，并测试保留策略。

```bash
make backup
```

## 恢复演练

恢复只能写入全新的临时数据库，先验证 schema version、核心表数量、账本和记录数，再决定切换；不能覆盖仍在服务的权威库。

```bash
make backup-restore-check
```

## 失败与回滚

dump 失败时停止发布；恢复校验失败时保留原库和备份，不切流。RPO/RTO 取决于部署环境的备份频率与数据库大小，本地脚本不构成生产承诺。
