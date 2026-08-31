// Command domaincheck rejects floating-point truth in financial domain code.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

type finding struct {
	Path   string
	Line   int
	Column int
	Type   string
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: domaincheck <go-package-directory>")
		os.Exit(2)
	}
	findings, err := findForbiddenFloatTypes(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "domain numeric check failed")
		os.Exit(1)
	}
	for _, finding := range findings {
		fmt.Fprintf(os.Stderr, "%s:%d:%d: forbidden domain numeric type %s\n", finding.Path, finding.Line, finding.Column, finding.Type)
	}
	if len(findings) > 0 {
		os.Exit(1)
	}
}

func findForbiddenFloatTypes(root string) ([]finding, error) {
	var findings []finding
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}

		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			var forbiddenType string
			switch value := node.(type) {
			case *ast.Ident:
				if value.Name == "float32" || value.Name == "float64" {
					forbiddenType = value.Name
				}
			case *ast.BasicLit:
				if value.Kind == token.FLOAT {
					forbiddenType = "floating-point literal"
				}
			}
			if forbiddenType != "" {
				position := fileSet.Position(node.Pos())
				findings = append(findings, finding{
					Path:   position.Filename,
					Line:   position.Line,
					Column: position.Column,
					Type:   forbiddenType,
				})
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].Column < findings[j].Column
	})
	return findings, nil
}
