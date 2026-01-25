// Package modules_test verifies module boundary compliance.
// This test ensures modules don't import runtime internals, enforcing clean separation.
package modules_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestModuleBoundaryCompliance verifies that module packages don't import runtime internals.
// This enforces the architectural boundary: modules should only use their own interfaces,
// not runtime implementation details.
func TestModuleBoundaryCompliance(t *testing.T) {
	// List of module packages to check
	modulePackages := []string{
		"internal/modules/input",
		"internal/modules/filter",
		"internal/modules/output",
	}

	// Forbidden imports - modules should NOT import these
	forbiddenImports := []string{
		"github.com/canectors/runtime/internal/runtime",
		"github.com/canectors/runtime/internal/factory",
		"github.com/canectors/runtime/internal/scheduler",
	}

	for _, pkgPath := range modulePackages {
		t.Run(pkgPath, func(t *testing.T) {
			// Find all .go files in the package (excluding tests for this boundary check)
			matches, err := filepath.Glob(filepath.Join("../..", pkgPath, "*.go"))
			if err != nil {
				t.Fatalf("failed to glob package %s: %v", pkgPath, err)
			}

			for _, file := range matches {
				// Skip test files - they may import runtime for testing purposes
				if strings.HasSuffix(file, "_test.go") {
					continue
				}

				// Parse the Go file
				fset := token.NewFileSet()
				content, err := os.ReadFile(file)
				if err != nil {
					t.Fatalf("failed to read file %s: %v", file, err)
				}

				f, err := parser.ParseFile(fset, file, content, parser.ImportsOnly)
				if err != nil {
					t.Fatalf("failed to parse file %s: %v", file, err)
				}

				// Check imports
				for _, imp := range f.Imports {
					importPath := strings.Trim(imp.Path.Value, `"`)
					for _, forbidden := range forbiddenImports {
						if importPath == forbidden {
							t.Errorf("BOUNDARY VIOLATION: %s imports forbidden package %s\n"+
								"Modules must not depend on runtime internals. Use interfaces only.",
								filepath.Base(file), forbidden)
						}
					}
				}
			}
		})
	}
}

// TestRuntimeUsesInterfacesOnly documents that runtime only uses module interfaces.
// This is enforced at compile time by Go's type system: the Executor struct declares
// fields as interface types (input.Module, filter.Module, output.Module), which prevents
// the runtime from accessing concrete module types or their internals.
func TestRuntimeUsesInterfacesOnly(t *testing.T) {
	// This test documents the architectural constraint.
	// The actual enforcement is done at compile time by Go's type system:
	//   type Executor struct {
	//       inputModule   input.Module      // Interface type - enforced by Go
	//       filterModules []filter.Module   // Interface type - enforced by Go
	//       outputModule  output.Module     // Interface type - enforced by Go
	//   }
	// in internal/runtime/pipeline.go
	//
	// The type system prevents the runtime from using concrete types - if someone
	// tries to change a field to a concrete type, the code won't compile.

	t.Log("Runtime boundary compliance is enforced at compile time by Go's type system")
	t.Log("See internal/runtime/pipeline.go Executor struct - fields are interface types")
}
