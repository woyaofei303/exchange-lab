# Runbook：发布

## 前置条件

- 使用已通过 CI 与 Trivy 的不可变 commit-SHA image，不使用 `latest`。
- secret 文件存在且权限正确；域名/DNS/TLS 入口就绪。
- migration 兼容性已评审，备份恢复演练通过，当前没有 P1/P2 事故。

## 流程

1. 运行 `make release-check` 和 `make backup-restore-check`。
2. 执行备份；拉取指定 `IMAGE_TAG`。
3. 运行 migration job；失败立即停止，不启动新应用。
4. 启动服务，等待 dependency health 与 `/readyz`。
5. 运行 `deploy/smoke.sh` 和核心只读/交易验收。
6. 将成功 tag 写为 current，原 tag 保留为 previous；观察 SLO/业务不变量。

仓库脚本：

```bash
IMAGE_TAG=<40-character-commit-sha> ./deploy/release.sh
```

生产执行需要目标主机权限和真实 secret，本地配置检查通过不等于已发布公网。
