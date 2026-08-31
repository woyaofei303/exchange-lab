package identity

// Role is the persisted authorization role.
type Role string

// Supported roles; administrators include ordinary user permissions.
const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// Valid reports whether the role is recognized.
func (r Role) Valid() bool { return r == RoleUser || r == RoleAdmin }

// Allows reports whether the role satisfies the required role.
func (r Role) Allows(required Role) bool {
	return r.Valid() && required.Valid() && (r == RoleAdmin || required == RoleUser)
}
