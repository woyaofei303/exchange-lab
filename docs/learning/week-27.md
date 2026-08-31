# Week 27：CI/CD、备份与回滚

## 学习重点

- CI 门禁顺序、immutable commit SHA、镜像扫描、staging environment approval。
- 发布前备份、migration、health gate、smoke、current/previous release pointer。
- binary rollback、forward-fix migration 与数据库恢复的边界。

## 代码导读

阅读 `.github/workflows/ci.yml`、`deploy-staging.yml`、`deploy/release.sh`、`rollback.sh` 与备份脚本。

## 动手与验收

```bash
make backup-restore-check
make release-check
```

在隔离环境用两个存在的 commit-SHA image 演练 release/rollback。未验证备份、未通过 readiness、存在不可兼容 destructive migration 时必须停止发布。

复盘：写清恢复点目标、恢复时间目标、谁批准、如何验证资金与仓位不变量。
