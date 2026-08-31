package ai

import "testing"

func TestToolCatalogPermanentlyExcludesTradingWrites(t *testing.T) {
	for _, name := range []string{"place_order", "cancel_order", "adjust_margin", "change_leverage", "transfer"} {
		if IsReadOnlyTool(name) {
			t.Fatalf("write tool %q exposed", name)
		}
	}
	if len(ReadOnlyToolNames) < 5 {
		t.Fatal("missing required read tools")
	}
}
