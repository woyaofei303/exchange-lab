# 依赖决策登记

- PostgreSQL/pgx：交易与审计真值。故障时 readiness 失败、写入停止；备份后前向迁移或回退兼容镜像。
- Redis/go-redis：行情缓存、限流、denylist。市场读可回源，安全状态失败关闭；可清空重建。
- NATS/JetStream：至少一次事件和显式 ACK。失败时 Outbox 保留，恢复后发布；格式不兼容通过新 subject/consumer 版本迁移。
- goose：嵌入式 schema v1-v16。失败阻断服务启动；先备份，优先前向修复，down 只用于已验证版本。
- coder/websocket：行情输入和 K 线输出。断线按有界退避重连；领域协议不依赖库类型。
- React/Vite/TanStack Query/KLineChart：浏览器展示和服务端状态获取。后端是风险真值；替换库时保留 runtime decoder 和 E2E。
- Tailwind CSS/@tailwindcss/vite：仅在前端构建期生成原子样式，不持有业务数据。失败时前端构建停止；回滚时删除 Vite 插件、CSS import 和对应依赖，现有手写 CSS 仍可独立工作。
- Biome/Prettier/prettier-plugin-tailwindcss：仅开发和 CI 使用，分别负责代码检查、统一格式及 Tailwind 类名排序，不持有运行数据。失败时阻断质量门禁或提交；可删除 npm scripts、配置和对应依赖回到 TypeScript/Vite 原有门禁。
- Husky/lint-staged：仅管理本地 Git pre-commit 和暂存文件检查，不读取业务数据。失败时阻断本地提交但不影响构建产物；紧急时可用 `HUSKY=0` 绕过，回滚时移除 hook、prepare script 和依赖并恢复 Git hooksPath。
- OpenAI Go SDK：可选结构化风险解释。模型失败返回 503 但交易核心继续；去掉 key 即回退本地确定模型。
- MCP Go SDK：stdio 只读工具适配。进程失败不影响 API；移除 MCP 不影响业务数据。
- Eino：v1 学习并保留框架中立 Tool/Skill 边界，但没有把 Eino 加入运行依赖。原因是当前没有需要 agent workflow 的写/编排能力；回滚成本最低，也避免同一工具契约绑定两个框架。
- OpenTelemetry/Prometheus/Grafana/Tempo：可观测数据。失败不进入交易事务；可关闭 overlay 回到日志/health。
- Testcontainers/Playwright/k6/Toxiproxy/govulncheck：仅开发/CI。失败阻断验证，不进入运行时数据路径。

所有新增依赖必须回答：用途、数据所有权、失败模式和回滚路径。版本固定在 `go.mod`、`web/package.json` 或 Compose；不得使用生产 `latest` 标签。
