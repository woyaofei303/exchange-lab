# Week 01：Go 与容器化开发底座

## 本周目标

能从 `cmd/api/main.go` 追到装配、配置、健康检查和优雅关闭，并能在依赖宕机时判断进程是否仍存活、是否仍可接流量。

## 学习与代码

- Go：module、package、interface、context、`net/http`、signal。
- 容器：image、container、volume、network、healthcheck、Compose dependency condition。
- 主路径：`cmd/api` → `internal/app` → `internal/health` → `internal/httpserver`。

## 动手任务

1. 阅读 readiness 的成功/失败测试并手写响应契约。
2. 停止 Redis，比较 `/healthz` 与 `/readyz`。
3. 给一个 fake dependency 添加 100ms 超时测试，再恢复原代码。

## 验收

```bash
make check
API_HOST_PORT=18080 BASE_URL=http://127.0.0.1:18080 make up smoke
```

完成后应能回答：为什么依赖错误只记录固定状态、为什么应用关闭要先停止接收新请求、为什么日常 `down` 不删除数据卷。
