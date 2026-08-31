# 威胁模型 v1

## 资产与信任边界

- 资产：密码哈希、opaque session、用户身份/角色、模拟账户/订单以及审计记录。
- 浏览器、REST/WS payload 和转发 header 均不可信；Go API 是授权决策点。
- PostgreSQL 保存用户、session 哈希和审计真值；Redis 只保存有 TTL 的登录限流状态。
- AI 与 MCP 永久位于只读边界，不能获得 session、密码、原始 token 或交易写权限。

## 主要威胁与控制

### 凭据泄露与撞库

- 密码使用每用户随机 salt 的 Argon2id（19 MiB、2 次、单并行度），不保存明文或可逆密文。
- 登录按 email+来源地址的哈希 key 在 Redis 原子限流；Redis 不可用时认证失败关闭。
- 登录失败统一返回 401，不暴露邮箱是否存在；审计不记录邮箱、密码或 token。

### Session 窃取与固定

- token 使用 32 字节 CSPRNG，只在 cookie 传输；数据库只保存 SHA-256 token hash。
- cookie 设置 HttpOnly、SameSite=Strict、Path=/；生产环境必须设置 Secure。
- refresh 在 Serializable 事务内撤销旧 token 并创建新 token；logout 后旧 token 立即无效。

### CSRF、跨域与越权

- 写路由拒绝跨站 `Origin`/`Sec-Fetch-Site`，且 cookie 使用 SameSite=Strict。
- 用户身份和 role 每次从服务端 session 联表读取；不信任浏览器 role。
- 管理员审计路由由 `Role.Allows` 服务端检查；拒绝行为本身写审计。

### 注入、过载与信息泄露

- JSON body 限制为 4 KiB，拒绝未知字段与多段 JSON。
- SQL 只用参数化查询；列表 limit 最大 200。
- 外部错误映射为固定 code/message，日志不写密码、原始 session、DSN 或底层错误值。
- Argon2 参数从已存 hash 解码时有严格上限，防止恶意 hash 导致内存/CPU DoS。

### 交易、风控与重放

- 订单、保证金、强平和条件单依赖当前 session user ID，不能由 body 指定 owner。
- 金额/价格/数量使用 atoms + scale，边界拒绝 `float64` 真值、溢出、错误 tick/step 和负余额。
- Outbox + consumer dedupe 防止双写丢失与重复副作用；不得通过删除 dedupe/幂等事实重放。
- 强平使用质量合格的 mark price 和 risk version 幂等 job；last price 不能替代强平基准。
- funding 与 liquidation 的账本必须同资产净和为零，对账异常立即停止相关 writer。

### AI、MCP 与 prompt injection

- 模型和工具只获得当前用户、指定 symbol 的裁剪证据；不暴露 session、password、DSN、API key 或其他用户 payload。
- 输出必须符合 schema 且 evidence ID 属于 allowlist；自然语言中的“忽略规则/替我下单”不能提升权限。
- MCP 仅注册只读工具并记录固定 outcome 审计；不存在订单、保证金、资金或管理员写工具。
- provider 超时/失败时核心交易不受影响，前端显示确定性降级而不伪造风险数字。

### 容器、供应链与公网边界

- 生产只有 Caddy 暴露 80/443；后端网络 internal，应用 non-root/read-only/cap-drop，secret file 不进入环境明文。
- CI 固定依赖/action/image 版本，运行 Go vulnerability 和容器 scan；生产镜像使用不可变 commit SHA。
- 发布先备份、migration、health gate、smoke，回滚默认只回退兼容镜像，不自动执行 destructive schema down。

## 剩余风险

- 本地开发 `AUTH_COOKIE_SECURE=false` 仅允许 loopback HTTP；staging/prod 必须为 true 并只经 HTTPS。
- 当前没有邮箱验证、MFA 和设备/session 管理 UI；v1.0 明确记录限制。
- 来源地址可能经过代理；上线前只信任 Caddy 写入且来自内部网络的标准转发头。
- PostgreSQL 泄露后 Argon2id 仍可能遭离线破解；需要强密码、参数升级和未来 pepper/KMS 策略。
- 单节点 Compose 不提供生产 HA、PITR 或多地域灾备；公网环境还需主机加固、防火墙、备份加密、告警接收和密钥轮换。
- 模拟行情、简化风险 tier/ADL 和本地容量结果不能当成真实交易所安全或性能证明。
