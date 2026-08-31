# Week 05：Redis 缓存、限流与撤销状态

## 本周目标

本周把 Redis 从“可连接的基础设施”升级为有边界的应用组件：市场读取使用 cache-aside，Redis 故障不影响 PostgreSQL 权威读取；登录限流和 token denylist 则返回错误，供后续认证模块失败关闭。

## 代码导读

先从 `internal/market/cached_reader.go` 看 cache-aside 主流程：

1. 先读 Redis；命中后直接返回。
2. 未命中或 Redis 出错时读取权威 `Reader`。
3. 权威读取成功后回填 Redis；回填失败只记录脱敏告警。
4. 权威来源错误仍向调用者返回，不能用陈旧缓存掩盖业务故障。
5. 请求 context 已取消时立即退出，不继续访问下游。

`internal/platform/rediscache/market_cache.go` 负责 Redis DTO 和领域对象之间的转换。价格、数量和最小名义价值仍然使用 `atoms + scale`，缓存中没有 `float64`。

`internal/platform/rediscache/rate_limit.go` 使用 Lua 把 `INCR`、首次 `PEXPIRE` 和 TTL 读取变成一个原子操作，避免并发登录请求创建不带过期时间的计数器。

`internal/platform/rediscache/denylist.go` 只存 token ID 的 SHA-256，并让 key 与 token 同时过期。Redis 故障会返回错误；第 9 周认证层接入后必须据此拒绝请求。

完整 key、TTL、容量和故障策略见 `docs/cache-catalog.md`。

## 为什么使用 noeviction

同一 Redis 同时承载性能缓存与安全状态。若使用 LRU/LFU 淘汰，内存压力可能删除 denylist 或登录限流 key，使被撤销 token 重新有效或绕过限流。因此设置：

```text
maxmemory 128mb
maxmemory-policy noeviction
```

达到上限时写入会失败。市场读取仍回退权威数据库；安全操作则失败关闭。这比悄悄丢失安全状态更可控。

## TDD 记录

- RED：缓存命中、未命中、读写失败、context 取消行为没有实现；GREEN：实现 `CachedReader` 并用 fake source/cache 验证分支。
- RED：私有 key 会暴露原始身份；GREEN：规范化输入并使用 SHA-256，限制输入长度。
- RED：Redis 市场缓存、限流和 denylist 类型不存在；GREEN：实现模块并用 Testcontainers Redis 验证真实 TTL 与故障行为。
- RED：配置没有缓存 timeout/TTL；GREEN：新增严格正时长配置和测试。
- RED：Compose 校验允许不安全淘汰策略；GREEN：固定并校验 `128mb + noeviction`。
- RED：Redis 清空或停止后的权威读取未验证；GREEN：集成测试与 acceptance 分别验证 FlushDB、容器停止和恢复。

## 动手练习

1. 运行单元、race、领域检查和构建：

```bash
make check
```

2. 启动独立 PostgreSQL/Redis Testcontainers 集成测试：

```bash
make integration
```

3. 本机 8080 已被占用时，在 18080 完成运行态验收：

```bash
API_HOST_PORT=18080 BASE_URL=http://127.0.0.1:18080 make up smoke acceptance
```

4. 查看 Redis 策略和业务 key：

```bash
docker compose exec -T redis redis-cli CONFIG GET maxmemory maxmemory-policy
docker compose exec -T redis redis-cli --scan --pattern 'exchange-lab:v1:*'
```

5. 阅读测试并手动预测每个断言：

```text
internal/market/cached_reader_test.go
internal/platform/rediscache/redis_integration_test.go
internal/platform/rediscache/keys_test.go
```

## 验收结果

- [x] cache hit 不访问权威来源。
- [x] cache miss 回源并带 TTL 回填。
- [x] Redis 清空后不丢失 PostgreSQL 权威数据。
- [x] Redis 停止时 `/v1/instruments` 仍返回 PostgreSQL 数据。
- [x] 登录限流在并发安全的原子脚本中计数并过期。
- [x] denylist 只保存摘要且与 token 同时过期。
- [x] 所有业务 key 都有命名空间和 TTL。
- [x] Redis 恢复后 API 无需重启即可重新 ready。

## 官方资料

- [Redis keyspace 与 TTL](https://redis.io/docs/latest/develop/using-commands/keyspace/)
- [Redis eviction 策略](https://redis.io/docs/latest/develop/reference/eviction/)
- [Redis 限流示例](https://redis.io/docs/latest/develop/use-cases/rate-limiter/)
- [Testcontainers for Go Redis 模块](https://golang.testcontainers.org/modules/redis/)
