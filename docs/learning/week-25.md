# Week 25：测试、性能与故障演练

## 学习重点

- unit、property、fuzz、race、integration、E2E、benchmark、load、chaos 的适用层级。
- latency percentile、吞吐、错误率、资源饱和和机器环境。
- 故障演练的稳态假设、注入范围、停止条件和恢复验证。

## 代码导读

阅读各 package 测试、`scripts/fuzz-smoke.sh`、`load/k6-contract.js`、`scripts/load-test.sh` 与 `fault-injection.sh`。

## 动手与验收

```bash
make fuzz-smoke
make load-test
BASE_URL=http://127.0.0.1:18080 make fault-injection
```

将机器、命令、commit、样本数、p95/p99、错误和瓶颈写入 `docs/performance-report.md`。不要把本地结果宣称为生产容量。
