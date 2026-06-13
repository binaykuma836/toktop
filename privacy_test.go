package main

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoNetworkImports enforces toktop's headline promise: it never makes a
// network call (and never shells out). If a dependency or new code ever imports
// one of these packages, "100% local" stops being true — and this test fails.
//
// Run it yourself: `go test -run TestNoNetworkImports ./...`
func TestNoNetworkImports(t *testing.T) {
	banned := map[string]string{
		"net":        "raw network sockets",
		"net/http":   "HTTP client/server",
		"net/url":    "URL handling (implies network)",
		"crypto/tls": "TLS connections",
		"os/exec":    "shelling out to other programs",
	}

	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "dist", "docs":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		for _, imp := range f.Imports {
			pkg := strings.Trim(imp.Path.Value, `"`)
			if why, bad := banned[pkg]; bad {
				t.Errorf("%s imports %q (%s) — toktop must stay 100%% local", path, pkg, why)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
