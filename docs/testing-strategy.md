# 测试策略

## 分层

- 纯单元/属性测试：fixed math、订单、撮合、仓位、保证金、强平、资金费率和保险不变量；最快、每次提交运行。
- handler contract：状态码、header、decimal string、错误脱敏、用户隔离；不替代 service/repository 测试。
- integration：Testcontainers 启动全新 PostgreSQL/Redis，验证 migration v1–v16、事务、约束、幂等、回滚和缓存故障。
- real-stack acceptance：Compose 的 API/workers/PostgreSQL/Redis/NATS 串联下单、成交、仓位、风险、资金费率、强平与对账。
- browser E2E：真实 Web gateway/API 的双用户开仓、刷新恢复和平仓。
- race/fuzz：并发数据竞争、协议 decoder 与核心数值/状态机异常输入。
- benchmark/load：测纯核心成本与 HTTP 合约吞吐；记录机器和目标，不外推生产容量。
- fault injection：短暂停 NATS，验证 readiness 降级、服务恢复和数据没有被清空。

## 门禁顺序

先格式/静态/契约，再 unit/race/build/frontend，再 integration/acceptance/E2E，最后 backup/security/image scan。任何账本净和、幂等或权限隔离失败都阻止发布。

## 可复查命令

```bash
make check
make integration
make backup-restore-check
BASE_URL=http://127.0.0.1:18080 make acceptance matching-check derivatives-check web-e2e
make fuzz-smoke
make load-test
BASE_URL=http://127.0.0.1:18080 make fault-injection
make security-check release-check
```

测试生成物放在 `output-tdd/`，不提交仓库。失败时先保存命令、commit、时间和日志，再做最小复现；不得通过删除数据库、dedupe 或测试断言让门禁“变绿”。
