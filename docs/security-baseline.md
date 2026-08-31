# 生产安全基线

## 已实现控制

- 应用 multi-stage 构建，runtime 固定 `USER 10001:10001`、read-only filesystem、`cap_drop: ALL`、`no-new-privileges` 和进程/内存/CPU 上限。
- 生产 Compose 只有 Caddy 暴露 80/443；PostgreSQL、Redis、NATS、API、workers、Grafana 和 Prometheus 不直接发布宿主机端口。
- PostgreSQL DSN、数据库密码和 OpenAI key 通过 Docker secret file 注入；脚本不打印内容。
- 浏览器认证使用 HttpOnly/SameSite cookie、Argon2id、opaque session、token hash、origin 检查、Redis fail-closed 登录限流。
- 金额/价格/数量使用整数 atoms；所有资金事实保存在 PostgreSQL 复式账本并可对账。
- CI 执行 Go vulnerability check 与容器 image scan；HTTP 返回安全响应头和固定脱敏错误。
- AI/MCP 只读、用户隔离、结构化输出和审计，不拥有交易写工具。

## 部署前必须由环境提供

- 独立 Linux 主机或受管容器平台、受限 SSH/runner 身份、域名与正确 DNS。
- GitHub `staging` environment approval、GHCR 读取权限和 repository/environment secrets。
- secret 文件权限 `0400`，密钥轮换机制，数据库/备份加密和异地保留。
- 云防火墙只放行 80/443 与受控管理入口；数据库和监控不开放公网。
- 告警接收人、on-call、时间同步、日志留存、主机补丁和入侵检测。

## 验证

```bash
make security-check
make release-check
```

`security-check` 依赖 Docker Compose、`jq` 和 Go toolchain。通过只证明仓库配置满足基线，不替代主机、云账户、DNS、证书和运行时渗透测试。

## 回滚

安全配置回退必须保留 secret file、non-root、端口最小化和 TLS 边界。若新版本无法在这些限制下运行，停止发布并修复镜像，不允许临时改成 root、明文 secret 或开放数据库端口。
