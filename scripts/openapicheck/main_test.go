package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryOpenAPISpecIsValid(t *testing.T) {
	t.Parallel()

	if err := validateSpecFile("../../openapi.yaml"); err != nil {
		t.Fatalf("validateSpecFile() error = %v", err)
	}
}

func TestValidateSpecFileRejectsInvalidDocument(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "invalid.yaml")
	content := []byte("openapi: 3.0.3\ninfo:\n  title: missing version\npaths: {}\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := validateSpecFile(path); err == nil {
		t.Fatal("validateSpecFile() error = nil")
	}
}
