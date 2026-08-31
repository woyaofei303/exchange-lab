# Week 22：只读 AI 风险助手

## 学习重点

- schema-constrained output、证据引用、工具输入授权、超时和 deterministic fallback。
- prompt injection、跨用户数据泄漏、幻觉数值、敏感日志等威胁。
- eval set 如何覆盖正常、边界、恶意和模型不可用场景。

## 代码导读

阅读 `internal/ai/models.go`、`service.go`、`tool_catalog.go`、`eval_test.go` 和 `internal/httpserver/ai.go`。

## 动手与验收

增加一个恶意输入样本和一个缺失证据样本；验证回答不能引入工具输出外的风险数字，也不能产生任何交易写入。

```bash
go test ./internal/ai
BASE_URL=http://127.0.0.1:18080 make derivatives-check
```

复盘：说明 AI 回答与权威风险计算的边界，模型不可用时用户仍能看到什么。
