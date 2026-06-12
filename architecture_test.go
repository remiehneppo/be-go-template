package be_go_template_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoMongoDriverImportsLeakOutsideRepositories(t *testing.T) {
	roots := []string{
		filepath.Join("internal", "app"),
		filepath.Join("internal", "domain"),
		filepath.Join("internal", "handler"),
	}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("ParseFile(%s) error = %v", path, err)
			}
			for _, spec := range file.Imports {
				importPath := strings.Trim(spec.Path.Value, `"`)
				if importPath == "go.mongodb.org/mongo-driver/v2/bson" || importPath == "go.mongodb.org/mongo-driver/v2/mongo" || importPath == "go.mongodb.org/mongo-driver/v2/mongo/options" {
					t.Fatalf("mongo driver import leaked outside repository layer: %s imports %s", path, importPath)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("WalkDir(%s) error = %v", root, err)
		}
	}
}
