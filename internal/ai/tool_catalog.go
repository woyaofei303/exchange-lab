package ai

// ReadOnlyToolNames 是 AI 层和 MCP 层共享的允许列表，禁止加入任何会修改交易状态的能力。
var ReadOnlyToolNames = []string{
	"get_position",
	"get_margin_status",
	"get_mark_price",
	"get_funding",
	"get_liquidation_history",
}

// IsReadOnlyTool 表示工具是否位于明确的只读允许列表中。
func IsReadOnlyTool(name string) bool {
	for _, allowed := range ReadOnlyToolNames {
		if name == allowed {
			return true
		}
	}
	return false
}
