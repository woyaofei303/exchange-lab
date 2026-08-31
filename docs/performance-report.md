# 性能与鲁棒性基线

## 环境与限制

- 已记录环境：Apple M1 Pro 本地开发机，Docker/Colima；结果仅用于回归基线。
- benchmark 是纯 Go 核心函数，不含网络、数据库、NATS 或浏览器成本。
- k6 结果应从 `output-tdd/performance/k6-contract.txt` 读取；每次硬件、commit 和容器资源变化后重跑，不把本地 QPS 当作生产容量承诺。

## 已执行结果

- fuzz smoke：marketdata 187,110 次、matching 370,887 次、position 1,174,461 次执行，未发现 crash/invariant failure。
- k6：10 VUs、30 秒，2,940 个 HTTP 请求，97.44 req/s，失败率 0%，p95 4ms；ticker 和 risk price 检查 2,940/2,940 通过。
- matching benchmark：81.692µs/op，98,307 B/op，746 allocs/op。
- position apply：58.44ns/op，56 B/op，2 allocs/op。
- isolated margin：191.2ns/op，224 B/op，8 allocs/op。
- liquidation evaluate：168.6ns/op，96 B/op，4 allocs/op。

数字来自 2026-08-10 的一次本地运行，测量噪声和代码变化会使结果不同。权威证据以当前 `output-tdd/performance` 文件为准。

## 重跑

```bash
make fuzz-smoke
make load-test
```

## 下一步诊断顺序

1. 先看 HTTP p95/p99、错误率和 CPU/内存是否饱和。
2. 用 trace 区分 handler、PostgreSQL、Redis、NATS 和序列化时间。
3. 核对 worker lag、数据库锁等待和 query plan。
4. 只有 profiling 证明瓶颈后才优化；不能为提高数字破坏确定性、幂等或资金正确性。
