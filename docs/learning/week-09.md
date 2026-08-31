# 第 9 周：身份认证、RBAC 与审计

## 本周结果

注册、登录、session refresh/logout、当前用户与管理员审计查询已形成完整服务端闭环。密码只保存 Argon2id 哈希，随机 session 只把 SHA-256 hash 存入 PostgreSQL，Redis 登录限流失败时认证失败关闭。

## 建议阅读顺序

1. `internal/identity/password.go`：参数、salt、编码和常量时间校验。
2. `internal/identity/service.go`：注册、统一登录失败、session 轮换和 RBAC。
3. `internal/platform/postgres/identity_repository.go`：唯一约束与 refresh 事务。
4. `internal/httpserver/identity.go`：cookie、CSRF、body 上限和固定错误。
5. `migrations/00003_identity.sql`：role/session 约束和索引。
6. `docs/threat-model.md`：按资产、入口、攻击者、控制和剩余风险复盘。

## 关键知识

- Hash 不等于快速 SHA-256：密码使用 memory-hard Argon2id；session 的高熵随机 token 才适合用 SHA-256 索引。
- 浏览器只持有 opaque token，不持有 user ID/role 的可信声明；服务端每次联表获取最新角色与状态。
- refresh 是旋转，不是延长旧 token；旧 token 撤销和新 token 插入必须同一事务。
- `SameSite=Strict` 不是完整 CSRF 策略，所以写操作还检查 Origin/Sec-Fetch-Site。
- Redis 限流属于安全状态，故障时不能绕过登录限制。

## 练习

1. 增加列出/撤销其他设备 session 的 API，但永远不返回 token hash。
2. 为管理员账号增加 MFA 状态和 step-up authentication 设计。
3. 测量本机 Argon2id p95，调整参数时保留兼容旧 hash 的验证路径。
4. 用测试证明 audit metadata 无法写入 password、token、authorization 等敏感 key。

## 官方资料

- [Go Argon2](https://pkg.go.dev/golang.org/x/crypto/argon2)
- [OWASP Password Storage](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)
- [OWASP Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)
- [Go HTTP Cookie](https://pkg.go.dev/net/http#Cookie)
