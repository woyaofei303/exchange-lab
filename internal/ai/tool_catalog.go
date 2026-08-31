package ai

// ReadOnlyToolNames is an allow-list shared by the AI and MCP layers. A
// mutating trading capability must never be added to this list.
var ReadOnlyToolNames = []string{
	"get_position",
	"get_margin_status",
	"get_mark_price",
	"get_funding",
	"get_liquidation_history",
}

// IsReadOnlyTool reports whether a tool is in the explicit non-mutating allow-list.
func IsReadOnlyTool(name string) bool {
	for _, allowed := range ReadOnlyToolNames {
		if name == allowed {
			return true
		}
	}
	return false
}
