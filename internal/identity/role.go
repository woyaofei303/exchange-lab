package identity

// Role 表示持久化的授权角色。
type Role string

// 支持的角色；管理员包含普通用户权限。
const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// Valid 表示角色是否可识别。
func (r Role) Valid() bool { return r == RoleUser || r == RoleAdmin }

// Allows 表示当前角色是否满足所需角色要求。
func (r Role) Allows(required Role) bool {
	return r.Valid() && required.Valid() && (r == RoleAdmin || required == RoleUser)
}
