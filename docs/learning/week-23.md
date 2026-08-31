# Week 23：MCP、Skills 与 AI 辅助研发

## 学习重点

- read-only tool contract、输入校验、最小字段、用户隔离、timeout、audit。
- 项目 Skill 的触发条件、固定检查清单和渐进式上下文。
- AI 编码的闭环：规则 → 计划 → RED → GREEN → review → 验收。

## 代码导读

阅读 `internal/readtools`、`cmd/mcp-server`、migration 00016，以及 `skills` 下的十一个项目 Skill。

## 动手与验收

使用一个风险 Skill 审查边界，再通过 MCP 调用只读仓位/风险工具，核对 `tool_audit_logs` 中没有 secret 或完整 payload。

```bash
go test ./internal/readtools ./internal/ai
go run ./cmd/mcp-server -help
```

复盘：记录 AI 帮你减少了哪些机械代码，以及哪些资金/风控判断必须由测试和人工负责。
