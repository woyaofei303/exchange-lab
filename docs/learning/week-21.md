# Week 21：完整合约交易终端

## 学习重点

- React 认证态、下单、委托、仓位、风险、保证金、资金费率和错误恢复。
- TanStack Query cache/invalidation、运行时 decoder、cookie session 和 decimal string。
- 双用户浏览器 E2E 如何验证真实后端，而不是 mock 页面。

## 代码导读

阅读 `web/src/api.ts`、`contract.ts`、`TradingPanel.tsx` 和 `web/tests/contract-flow.spec.ts`。

## 动手与验收

在两个浏览器 context 中挂 maker/taker，刷新恢复仓位，再 reduce-only 平仓；观察 API、worker 与 UI 状态。

```bash
npm test --prefix web
BASE_URL=http://127.0.0.1:18080 npm run test:e2e --prefix web
```

复盘：列出所有 loading/error/empty/reconnecting 状态，避免只测试 happy path。
