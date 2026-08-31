# Week 24：可观测性与 SLO

## 学习重点

- HTTP RED、worker lag、parked events、账本不平衡、强平/资金费率和 AI 指标。
- trace context、Prometheus alert、Grafana dashboard、SLO 与错误预算。
- 从症状到证据的 incident diagnosis，而不是先重启。

## 代码导读

阅读 `internal/observability`、`observability/prometheus.yml`、`alerts.yml`、四个 dashboard 和 `docs/slo.md`。

## 动手与验收

启动 observability overlay，制造 NATS 短暂中断，检查 readiness、metric、alert 和恢复。

```bash
API_HOST_PORT=18080 docker compose -f compose.yaml -f compose.observability.yaml up -d --wait
curl --fail --silent http://127.0.0.1:9090/-/ready
curl --fail --silent http://127.0.0.1:3001/api/health
BASE_URL=http://127.0.0.1:18080 make fault-injection
```

复盘：写出检测、定位、缓解、恢复、复盘五步证据。
