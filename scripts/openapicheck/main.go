// Command openapicheck validates the local OpenAPI document without remote refs.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/getkin/kin-openapi/openapi3"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: openapicheck <openapi-file>")
		os.Exit(2)
	}
	if err := validateSpecFile(os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "OpenAPI validation failed: %v\n", err)
		os.Exit(1)
	}
}

func validateSpecFile(path string) error {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	document, err := loader.LoadFromFile(path)
	if err != nil {
		return fmt.Errorf("load specification: %w", err)
	}
	if err := document.Validate(context.Background()); err != nil {
		return fmt.Errorf("validate specification: %w", err)
	}
	return nil
}
