# 第 8 周：React 交易终端外壳与全栈联调

## 本周结果

通过 `http://127.0.0.1:3000` 打开只读永续合约终端：交易对、ticker、K 线、模拟深度与连接状态均来自正在运行的 Go 链路；历史 K 线来自 REST，实时 bar 来自 WebSocket。

## 建议阅读顺序

1. `web/src/contract.ts`：先看不可信 JSON 如何变成可信类型。
2. `web/src/api.ts`：看 fetch 错误、AbortSignal、时间窗口上限。
3. `web/src/candle-stream.ts`：看连接状态、封顶指数退避和 cleanup。
4. `web/src/KLinePanel.tsx`：看 KLineChart DataLoader 如何衔接 REST 与 WebSocket。
5. `web/src/App.tsx`：看 TanStack Query 的 identity key、加载态和 UI 状态。
6. `web/tests/terminal.spec.ts`：看停止/启动真实 API 后页面如何证明恢复。

## 重点知识

### 契约边界

TypeScript 类型不会验证运行时 JSON。`decode*` 函数检查 envelope、字段类型、版本、symbol、decimal 与时间；不能用 `response.json() as T` 绕过边界。金额字段为数字时测试必须失败。

### 历史与实时衔接

KLineChart 10 的 `getBars` 初始化历史，`subscribeBar` 每次推一个 bar，重复 timestamp 覆盖最后一条。`unsubscribeBar` 在切 symbol、period 或销毁时关闭旧 socket，防止旧行情污染新页面。

### 精度

Go 领域和公开契约使用 atoms + scale / decimal string。图表库要求 `number`，所以只在绘图适配器做近似转换。模拟深度以 `BigInt` 计算 atoms，避免在页面业务逻辑扩散二进制浮点。

### 前端数据状态

TanStack Query 的 query key 包含 symbol；WebSocket 状态是独立状态机。加载失败、历史失败、连接中、实时、重连中和离线都有不同表现，不把失败值显示成 `0`。

## 练习

1. 给 decoder 添加“high 小于 low”拒绝测试并实现 OHLC 关系验证。
2. 在不修改后端真值的前提下增加 stale 标识：ticker 超过 3 秒未更新时显示黄色。
3. 新增另一个模拟合约，证明切 symbol 后旧 socket 不再更新页面。
4. 在 Playwright 中记录 API 停止到页面显示“重连中”、恢复到“实时”的耗时。

## 官方资料

- [React TypeScript](https://react.dev/learn/typescript)
- [Vite Guide](https://vite.dev/guide/)
- [TanStack Query](https://tanstack.com/query/latest/docs/framework/react/installation)
- [KLineChart Data Integration](https://klinecharts.com/en-US/guide/data-integration)
- [Playwright](https://playwright.dev/docs/intro)
