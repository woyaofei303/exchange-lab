# Week 28：v1.0 交付、演示与求职表达

## 学习重点

- 从产品范围、架构、ERD、核心时序、公式、SLO、威胁模型到 runbook 的整体表达。
- 用可验证事实描述个人贡献，不包装成生产交易所、部门负责人或真实资金系统。
- 演示失败恢复和已知限制，而不是只展示 UI happy path。

## 最终任务

1. 从干净 clone 按 README 启动。
2. 按 `docs/demo-script.md` 完成双用户成交、仓位、风险、资金费率、AI 和故障恢复。
3. 执行 `docs/runbooks/release.md` 与 `rollback.md` 的桌面演练。
4. 使用 `docs/career` 准备简历项目描述和系统设计问答。

## 验收

```bash
make check
make integration
make backup-restore-check
BASE_URL=http://127.0.0.1:18080 make derivatives-check web-e2e
make security-check release-check
```

完成标准：别人不依赖你的口头提示，仅靠代码和文档就能启动、验证、理解故障边界并安全停止系统。
