# Week 26：生产容器与安全基线

## 学习重点

- 多阶段构建、不可变镜像、non-root、read-only filesystem、capability 与 secret file。
- 只暴露 Caddy、TLS 自动化、内部网络和最小 egress。
- dependency/SCA/image scan、日志脱敏和安全回滚。

## 代码导读

阅读 `Dockerfile`、`compose.prod.yaml`、`.env.prod.example`、`deploy/Caddyfile` 和 `scripts/security-check.sh`。

## 动手与验收

```bash
make security-check
make release-check
```

再渲染 Compose JSON，检查非 Caddy 服务没有 `ports`，应用 image 用户为 `10001:10001`，DSN/API key 不在 environment 明文。

复盘：生产配置依赖 Linux 主机、域名、DNS 和 secret 文件；本地校验通过不等于已公网部署。
