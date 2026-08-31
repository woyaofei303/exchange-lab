# 第 2 周：Go、TDD 与交易领域类型

## 本周结果

通过代码理解 Go 的 struct、method、error、测试 helper、表驱动测试和标准库 AST，并建立交易系统最基础的不变量：价格、数量和金额必须是带明确 scale 的精确十进制值。

## 建议学习顺序

1. 阅读 `internal/trading/asset.go`，理解构造函数如何保护 struct 不变量。
2. 阅读 `internal/trading/fixed.go`，手算 `atoms` 与 `scale` 的转换。
3. 阅读 `price.go`、`quantity.go` 和 `money.go`，比较三个类型为什么不能合并成一个通用数字。
4. 阅读 `instrument.go`，理解深模块如何把精度、步长和最小下单额规则集中在一个接口后面。
5. 对照每个 `*_test.go`，按测试名称复盘 RED→GREEN。
6. 阅读 `scripts/domaincheck`，理解 Go AST 如何识别真实类型标识符与浮点字面量。
7. 阅读 `docs/adr/ADR-001-fixed-point-domain-values.md`，用自己的话解释决策与代价。

## 必须掌握的 Go 知识

### struct 与构造函数

`Asset`、`Price`、`Quantity`、`Money` 和 `Instrument` 都把字段设为私有。调用方必须使用构造函数或解析方法，不能绕过校验直接写字段。

重点问题：

- Go 的零值什么时候有用，什么时候必须 fail closed？
- 返回值使用 struct 而不是指针有什么影响？
- 私有字段如何缩小模块接口并提高可维护性？

### method 与领域语义

`Price.Atoms()` 与 `Quantity.Atoms()` 返回的都是 `int64`，但 `Price` 和 `Quantity` 仍然是不同类型。这样可以防止把数量误当价格传入，同时让方法名称表达业务语义。

### pointer、interface 与依赖注入

- 领域值对外按值传递，避免调用方共享可变状态；`math/big.Int` 只作为乘法的内部临时值，通过指针调用其可变方法，但不会泄漏到模块接口。
- 本周没有为 `Price`、`Quantity` 或 `Instrument` 创建 Go interface，因为它们只有一种实现，没有真实可替换的 seam。为了测试而创建单实现 interface 会让模块变浅。
- 本周领域计算是纯函数，不访问数据库、时钟或网络，因此不需要依赖注入。第 3 周接入 transport/store 时，再把真实可变依赖从外部传入。

### error 与 errors.Is

领域错误使用 sentinel error，例如：

```text
ErrPrecisionExceeded
ErrScaleMismatch
ErrStepMismatch
ErrBelowMinimumNotional
ErrOverflow
```

实现代码使用 `%w` 添加上下文，测试和调用方使用 `errors.Is` 判断错误类别，不依赖完整错误字符串。

### 测试 helper 与表驱动测试

`test_helpers_test.go` 中的 helper 负责构造测试前置条件，并调用 `t.Helper()`，让失败行定位到真正的测试断言。

表驱动测试适合验证：

- 多种非法十进制输入；
- 多种非法合约配置；
- 枚举的全部合法值；
- 边界值、精度值和溢出值。

## 固定小数手算

规则：

```text
真实值 = atoms / 10^scale
```

示例：

```text
50000.10，scale=2  => atoms=5000010
0.001，scale=3     => atoms=1
50000.10 × 0.001   => 50.00010
USDT scale=8       => money_atoms=5000010000
```

请自行完成以下练习：

1. 将 `123.4567` 转换为 scale=4 的 atoms。
2. 判断 `123.4567` 能否无损转换为 scale=2。
3. 计算 `2500.25 × 0.012`，并转换为 USDT scale=8。
4. 解释为什么 `price_atoms * quantity_atoms` 可能在两个操作数都未溢出时发生中间溢出。

## Instrument 的三个校验

### 精度

输入小数位不能超过 `PriceScale` 或 `QuantityScale`。模块不会静默截断。

### 步长

相同 scale 下使用整数取模判断：

```text
value_atoms % step_atoms == 0
```

### 最小下单额

先精确计算 `price × quantity`，再与 `MinNotional` 的 atoms 比较。等于最小值允许通过，小于最小值返回领域错误。

## TDD 练习

按以下顺序自行修改测试，并观察 RED 与 GREEN：

1. 把 BTCUSDT 的价格步长改为 `0.50`，为 `50000.10` 写一个失败测试。
2. 增加 ETHUSDT 合约测试，使用不同的数量 scale。
3. 为 `ParseMoney` 增加最大 `int64` 边界测试。
4. 尝试在 `internal/trading` 写入 `type UnsafePrice float64`，运行静态检查并观察失败，然后撤回练习代码。

## AI 协作检查清单

让 AI 生成或修改交易数值代码时，必须追问：

- 是否引入 `float32`、`float64` 或浮点字面量？
- 每个整数的单位和 scale 是什么？
- 乘法中间值是否可能溢出？
- scale 变化是否发生隐式舍入？
- 错误是否可由 `errors.Is` 稳定判断？
- 是否为了复用而引入失去语义的 `interface{}` 或通用数字类型？

本周明确拒绝的 AI 常见方案：

- 用 `float64` 表示价格和金额；
- 先将字符串转换为浮点数，再乘以 `10^scale`；
- 用一个通用 `Decimal` 类型替代 Price、Quantity、Money 的领域语义；
- 除法不能整除时自动四舍五入；
- 用字符串比较或分散在 handler 中重复校验。

## 验证命令

```bash
cd /Users/julian/exchange-lab
go test -count=1 ./internal/trading
go test -race -count=1 ./internal/trading
go test -cover ./internal/trading
make domain-check
make check
```

## 本周验收

- [ ] 能解释 atoms、scale 和真实值的关系。
- [ ] 能解释为什么账务真值不能使用二进制浮点数。
- [ ] 能手算精度、步长和最小下单额边界。
- [ ] 能为新边界先写失败测试，再实现最小修复。
- [ ] 能说明 Go 与 PostgreSQL 为什么必须共享明确的 scale。
- [ ] 所有领域测试、race 测试、静态检查和项目回归通过。
