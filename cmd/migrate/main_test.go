package main

import "testing"

func TestParseDirectionAcceptsOnlyBoundedMigrationOperations(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"up", "down", "version"} {
		value := value
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			if got, err := parseDirection(value); err != nil || string(got) != value {
				t.Fatalf("parseDirection(%q) = %q, %v", value, got, err)
			}
		})
	}
	for _, value := range []string{"", "reset", "up-to", "UP"} {
		value := value
		t.Run("reject_"+value, func(t *testing.T) {
			t.Parallel()
			if _, err := parseDirection(value); err == nil {
				t.Fatalf("parseDirection(%q) error = nil", value)
			}
		})
	}
}
