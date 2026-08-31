package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindForbiddenFloatTypesReportsTypeIdentifiers(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	writeGoFile(t, directory, "price.go", "package trading\ntype Price float64\n")

	findings, err := findForbiddenFloatTypes(directory)
	if err != nil {
		t.Fatalf("findForbiddenFloatTypes() error = %v", err)
	}
	if got, want := len(findings), 1; got != want {
		t.Fatalf("len(findings) = %d, want %d: %#v", got, want, findings)
	}
	if got, want := findings[0].Type, "float64"; got != want {
		t.Fatalf("finding type = %q, want %q", got, want)
	}
}

func TestFindForbiddenFloatTypesIgnoresCommentsAndStrings(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	writeGoFile(t, directory, "safe.go", "package trading\n// float64 is forbidden here.\nconst rule = `float32`\n")

	findings, err := findForbiddenFloatTypes(directory)
	if err != nil {
		t.Fatalf("findForbiddenFloatTypes() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestFindForbiddenFloatTypesReportsFloatingPointLiteral(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	writeGoFile(t, directory, "price.go", "package trading\nconst Price = 0.1\n")

	findings, err := findForbiddenFloatTypes(directory)
	if err != nil {
		t.Fatalf("findForbiddenFloatTypes() error = %v", err)
	}
	if got, want := len(findings), 1; got != want {
		t.Fatalf("len(findings) = %d, want %d: %#v", got, want, findings)
	}
	if got, want := findings[0].Type, "floating-point literal"; got != want {
		t.Fatalf("finding type = %q, want %q", got, want)
	}
}

func writeGoFile(t *testing.T, directory, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
