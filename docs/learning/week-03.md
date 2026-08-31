# Week 03：HTTP API、OpenAPI 与请求上下文

## 本周目标

把 Week 02 的永续合约领域对象放到稳定的只读 HTTP 边界后面，并形成可以由测试、OpenAPI 和 Docker 共同验证的接口契约。

本周不写订单、账户、撮合或真实资金功能。`/v1/instruments` 与 `/v1/tickers` 都是只读接口，行情仍由内存适配器提供。

## 完成结果

- `GET /healthz`：只检查进程存活。
- `GET /readyz`：检查 PostgreSQL、Redis 与 NATS。
- `GET /v1/instruments`：返回永续合约精度、步长和最小名义价值。
- `GET /v1/tickers`：返回字符串编码的只读行情快照。
- `symbol` 仅接受规范形式，例如 `BTCUSDT-PERP`。
- 所有响应带 `X-Request-ID`；错误使用统一 JSON 结构。
- 每个请求有截止时间、结构化日志和脱敏错误映射。
- `openapi.yaml` 由 `make openapi-check` 解析并做语义校验。
- Docker 构建上下文显式包含 API 需要的 Go 包，其他文件默认拒绝进入镜像。

## 建议阅读顺序

1. `internal/market/reader.go`：先理解 `Reader` 接口如何隔离 HTTP 和数据来源。
2. `internal/httpserver/handler.go`：看路由、参数验证、DTO 映射和统一错误。
3. `internal/httpserver/middleware.go`：看请求 ID、超时、日志和状态码记录。
4. `internal/app/run.go`：看依赖注入、服务器超时和优雅关闭。
5. `openapi.yaml`：对照代码阅读公开契约。
6. `internal/httpserver/handler_test.go`：从测试理解成功、失败、取消和超时语义。

## 请求链路

```text
HTTP client
    -> requestLogging（校验/生成 request ID，记录状态与耗时）
    -> withRequestTimeout（创建带 deadline 的 context）
    -> ServeMux（选择精确路径）
    -> getOnly（拒绝非 GET 请求）
    -> 参数验证
    -> market.Reader（业务读取端口）
    -> DTO 映射
    -> JSON 响应
```

## 核心知识

### 1. Handler、Middleware 与依赖注入

`http.Handler` 只负责传输协议。它依赖抽象的 `market.Reader`，不知道行情来自内存、PostgreSQL、Redis 还是远程服务。替换数据来源时无需重写路由。

这种依赖方向是：

```text
HTTP transport -> business port <- adapter
```

测试可以注入假的 reader，精确制造内部错误、取消和超时，不需要启动数据库。

### 2. Context 的取消与截止时间

每个市场接口调用都传入 `r.Context()`。客户端断开、上游取消或服务端截止时间到达时，下游必须尽快返回：

- `context.Canceled` 映射为 HTTP `408` 和 `request_canceled`。
- `context.DeadlineExceeded` 映射为 HTTP `504` 和 `request_timeout`。
- 其他内部错误映射为 HTTP `500`，不把底层错误写入响应或日志。

后续数据库查询必须继续使用带 context 的 `Query`/`Exec` 方法，不能换回无期限等待。

### 3. 结构化日志与请求 ID

服务只接受 1 到 64 位安全字符组成的客户端请求 ID；不安全的值会被替换。日志记录：

```text
request_id method path status duration_ms
```

日志只记录 `URL.Path`，不记录查询字符串和 `Authorization`。这样既能关联问题，也避免把 token、密码或其他输入写进日志。

### 4. 为什么 JSON 小数使用字符串

价格、数量、资金和盈亏不能把二进制浮点作为事实来源。API 中的小数都以字符串传输，例如：

```json
{
  "last_price": "50000.00",
  "volume_24h": "1234.567"
}
```

客户端能够保留精度和末尾零。真正进入交易计算前，字符串必须解析为 Week 02 的固定精度领域类型。

### 5. OpenAPI 是可执行契约

`openapi.yaml` 描述路径、方法、状态码、响应头、错误结构和字符串小数格式。`scripts/openapicheck` 关闭外部引用，只读取仓库中的规范，避免校验阶段访问未知 URL 或文件。

运行：

```bash
make openapi-check
```

修改 handler 时应同步修改规范和契约测试，否则 `make check` 不应通过。

### 6. 优雅关闭

收到进程取消信号后，服务器停止接收新连接，并在 `SHUTDOWN_TIMEOUT` 内等待正在处理的请求结束。`ReadHeaderTimeout`、`ReadTimeout`、`WriteTimeout` 和 `IdleTimeout` 分别限制不同阶段，不能用一个模糊的总超时替代。

## TDD 记录

本周按以下 RED -> GREEN 顺序推进：

1. 新 handler 依赖 `market.Reader` 时先出现编译失败，再补接口与注入。
2. `/v1/instruments` 先返回 404，再实现合约过滤器响应。
3. `/v1/tickers` 先返回 404，再实现字符串小数响应。
4. symbol 过滤测试先返回全部结果，再补精确过滤与输入校验。
5. 请求 ID 测试先得到空响应头，再加入生成、透传与安全替换。
6. 错误日志测试先缺少请求 ID，再注入请求级 logger。
7. 错误响应先不统一，再加入 `error.code/message/request_id`。
8. 截止时间先错误映射为 500，再区分 canceled、deadline 与内部错误。
9. 配置超时测试先耗时约 200ms，再加入请求 timeout middleware。
10. OpenAPI 校验器测试先因实现和规范缺失失败，再补校验器与规范。
11. 实际 Docker build 暴露新包未进入上下文，再补 `.dockerignore` 白名单。

## 本周练习

1. 增加第二个 `ETHUSDT-PERP` 合约，并验证两个列表及单 symbol 查询。
2. 给 ticker reader 加入 50ms 人工延迟，观察 10ms 和 100ms 请求超时的差异。
3. 修改 OpenAPI 中 `price_scale` 最大值为 1，运行校验并思考：语义校验为何不会自动验证运行时示例？
4. 写一个测试确认错误响应的 `request_id` 与响应头完全一致。
5. 用 `curl -H 'X-Request-ID: learn-week-03'` 请求接口，再从日志定位同一请求。

## AI 协作检查清单

让 AI 修改 HTTP 层后，人工逐项确认：

- 是否新增了不必要的对外端口或 Docker volume。
- 是否把凭据、Authorization、查询参数或底层错误写进日志。
- 是否给网络、数据库和服务器设置了合理超时。
- 是否把 `context.Context` 一直传到最下游。
- 是否使用 `float32`/`float64` 表示价格、数量、资金、保证金或盈亏。
- OpenAPI、测试、实现和冒烟脚本是否一致。
- 错误是否可关联但已脱敏。

## 验收命令

```bash
make check
docker compose up -d --build --wait --wait-timeout 120
make smoke
docker compose down
```

手工请求示例：

```bash
curl -i -H 'X-Request-ID: learn-week-03' http://127.0.0.1:8080/v1/instruments
curl -i 'http://127.0.0.1:8080/v1/tickers?symbol=BTCUSDT-PERP'
curl -i 'http://127.0.0.1:8080/v1/tickers?symbol=invalid'
```

## 验收标准

- [x] `make check` 通过。
- [x] 实际 Docker 镜像构建成功。
- [x] 四个 Compose 服务健康。
- [x] `/healthz`、`/readyz`、`/v1/instruments`、`/v1/tickers` 冒烟测试通过。
- [x] 非法参数、非 GET 方法、取消、超时和内部错误都有测试。
- [x] 每个响应都带请求 ID，日志不包含密码、token 或底层错误。
- [x] API 的交易小数仍为字符串，领域事实来源仍为固定精度整数。
