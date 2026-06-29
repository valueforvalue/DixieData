// Package architecture_test holds architectural boundary tests that
// run on every `go test ./...` invocation. The tests exist to catch
// architectural drift that an LLM or a hurried human might introduce:
// a domain package that suddenly starts importing Wails, a handler
// package reaching into another handler's internals, a route-builder
// starting to depend on the package whose routes it builds.
//
// The tests fail loudly with a file + line citation so the offender
// can fix the regression in one pass rather than chasing it through
// runtime symptoms.
package architecture_test

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Forbidden packages per protected package. The keys are path prefixes
// rooted at the repo root. The values are import paths that are not
// allowed for that package. Wildcards are not supported; add each
// forbidden path explicitly.
//
// Protected packages are the "deep module" layers that should stay
// free of UI/delivery concerns. The appshell, viewmodel, templates,
// presentation, debug packages are NOT protected — they are the
// delivery surfaces that legitimately touch everything.
//
// The viewmodel + presentation packages have a narrower forbidden
// list than the deep modules: they are the documented grey-box layer
// that converts deep-module DTOs into UI-ready shapes. They are
// forbidden from importing the delivery surface (appshell, templ,
// wails) but ARE allowed to import deeper modules like records,
// archive, models, jobs, update. The test enforces this boundary so
// a future contribution cannot quietly start importing templates or
// wails from viewmodel without breaking CI.
var forbiddenByPackage = map[string][]string{
	"internal/records": {
		"github.com/a-h/templ",
		"github.com/wailsapp/wails/v2",
		"github.com/valueforvalue/DixieData/internal/appshell",
		"github.com/valueforvalue/DixieData/internal/presentation",
		"github.com/valueforvalue/DixieData/internal/templates",
	},
	"internal/archive": {
		"github.com/a-h/templ",
		"github.com/wailsapp/wails/v2",
		"github.com/valueforvalue/DixieData/internal/appshell",
		"github.com/valueforvalue/DixieData/internal/presentation",
		"github.com/valueforvalue/DixieData/internal/templates",
	},
	"internal/db": {
		"github.com/a-h/templ",
		"github.com/wailsapp/wails/v2",
		"github.com/valueforvalue/DixieData/internal/appshell",
		"github.com/valueforvalue/DixieData/internal/presentation",
		"github.com/valueforvalue/DixieData/internal/templates",
	},
	"internal/models": {
		"github.com/a-h/templ",
		"github.com/wailsapp/wails/v2",
		"github.com/valueforvalue/DixieData/internal/appshell",
		"github.com/valueforvalue/DixieData/internal/presentation",
		"github.com/valueforvalue/DixieData/internal/templates",
	},
	"internal/appdata": {
		"github.com/a-h/templ",
		"github.com/wailsapp/wails/v2",
		"github.com/valueforvalue/DixieData/internal/appshell",
		"github.com/valueforvalue/DixieData/internal/presentation",
		"github.com/valueforvalue/DixieData/internal/templates",
	},
	"internal/dates": {
		"github.com/a-h/templ",
		"github.com/wailsapp/wails/v2",
		"github.com/valueforvalue/DixieData/internal/appshell",
		"github.com/valueforvalue/DixieData/internal/presentation",
		"github.com/valueforvalue/DixieData/internal/templates",
	},
	"internal/routebuilder": {
		// routebuilder must remain dependency-free so both appshell
		// (where routes are registered) and templates (where they're
		// referenced) can import it without closing the import cycle.
		// If you find yourself adding an import here, the route
		// probably belongs in appshell/routebuilder.go instead.
		"github.com/a-h/templ",
		"github.com/wailsapp/wails/v2",
		"github.com/valueforvalue/DixieData/internal/appshell",
		"github.com/valueforvalue/DixieData/internal/presentation",
		"github.com/valueforvalue/DixieData/internal/templates",
		"github.com/valueforvalue/DixieData/internal/uiids",
		"github.com/valueforvalue/DixieData/internal/htmxattr",
	},
	"internal/viewmodel": {
		// Grey-box layer: converts deep-module DTOs into UI shapes.
		// Forbidden from the delivery surface (appshell, templ,
		// wails, templates). Allowed to import records / archive /
		// models / jobs / update / dates / debug for the conversion.
		"github.com/a-h/templ",
		"github.com/wailsapp/wails/v2",
		"github.com/valueforvalue/DixieData/internal/appshell",
		"github.com/valueforvalue/DixieData/internal/templates",
	},
	"internal/presentation": {
		// Grey-box layer: renders the templ UI. templ itself is
		// allowed because presentation is the templ-rendering
		// adapter. Forbidden only from the HTTP handler shell
		// (appshell) and from the Wails runtime; a presentation
		// function reaching into an HTTP handler or a Wails
		// runtime call is a layering inversion.
		"github.com/wailsapp/wails/v2",
		"github.com/valueforvalue/DixieData/internal/appshell",
	},
}

// allowedInternalImportsPerPackage locks the pkg/* surface so each
// external package can only import the internal/ types it
// legitimately needs. Drift away from this allowlist fails
// TestPkgImportsAreAllowlisted.
//
// The allowlists below mirror the current imports; if a new pkg/
// file legitimately needs a new internal/ import, update the
// allowlist in the same commit that adds the import.
var allowedInternalImportsPerPackage = map[string]map[string]bool{
	"pkg/render": {
		"github.com/valueforvalue/DixieData/internal/models":  true,
		"github.com/valueforvalue/DixieData/internal/records": true,
	},
	"pkg/exportbridge": {
		"github.com/valueforvalue/DixieData/internal/archive": true,
		"github.com/valueforvalue/DixieData/internal/db":      true,
		"github.com/valueforvalue/DixieData/internal/models":  true,
	},
	"pkg/encode": {
		"github.com/valueforvalue/DixieData/internal/buildinfo": true,
		"github.com/valueforvalue/DixieData/internal/models":    true,
	},
	"pkg/templatespec": {
		// templatespec is documented to import nothing from
		// internal/. Empty allowlist is intentional.
	},
}

// TestPackageBoundaries walks every Go file under each protected
// package and fails if any import is in the forbidden list for that
// package. The failure message includes the file path and import
// spec position so the offender can fix it without re-reading the
// whole package.
func TestPackageBoundaries(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("locate repo root: %v", err)
	}

	for pkg, forbidden := range forbiddenByPackage {
		pkgDir := filepath.Join(repoRoot, pkg)
		if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
			t.Fatalf("protected package %q does not exist at %s", pkg, pkgDir)
		}

		fset := token.NewFileSet()
		err := filepath.WalkDir(pkgDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				name := d.Name()
				if name == "vendor" || name == "testdata" || (strings.HasPrefix(name, ".") && name != "." && name != "..") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}

			f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return fmt.Errorf("parse %s: %w", path, err)
			}

			relPath, _ := filepath.Rel(repoRoot, path)
			for _, imp := range f.Imports {
				importPath := strings.Trim(imp.Path.Value, `"`)
				for _, f2 := range forbidden {
					if importPath == f2 {
						pos := fset.Position(imp.Pos())
						t.Errorf("forbidden import in %s\n  file: %s:%d\n  package: %q\n  imports forbidden: %q\n  fix: move this import to a delivery-surface package (appshell/presentation/templates)",
							pkg, relPath, pos.Line, pkg, f2)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Errorf("walk %s: %v", pkg, err)
		}
	}
}

// findRepoRoot walks up from the current directory until it finds a
// go.mod file. Returns the directory containing go.mod.
func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found")
		}
		dir = parent
	}
}

// TestForbiddenPackagesAreDistinct is a sanity check: every forbidden
// list must have at least one entry, and no entry may be the empty
// string. Catches a typo in the table that would silently pass.
func TestForbiddenPackagesAreDistinct(t *testing.T) {
	for pkg, forbidden := range forbiddenByPackage {
		if len(forbidden) == 0 {
			t.Errorf("package %q has empty forbidden list; remove the entry", pkg)
		}
		seen := map[string]bool{}
		for _, f := range forbidden {
			if f == "" {
				t.Errorf("package %q has empty forbidden import path", pkg)
			}
			if seen[f] {
				t.Errorf("package %q has duplicate forbidden import %q", pkg, f)
			}
			seen[f] = true
		}
	}
}

// TestArchitectureMapsToContract locks in the architectural
// assertions made in AGENT_ARCHITECTURE_MAP.md and AGENTS.md. If
// this test fails, the architecture has drifted from the documented
// intent and the docs need updating in the same commit.
//
// Concretely: the test verifies that the boundary table above names
// every package listed in AGENTS.md as a "deep module" layer. If a
// new deep module is added (or an existing one is renamed), the
// test fails until the table is updated — keeping architecture and
// docs in sync.
func TestArchitectureMapsToContract(t *testing.T) {
	required := []string{
		"internal/records",
		"internal/archive",
		"internal/db",
		"internal/models",
		"internal/appdata",
		"internal/dates",
		"internal/routebuilder",
		"internal/viewmodel",
		"internal/presentation",
	}
	for _, pkg := range required {
		if _, ok := forbiddenByPackage[pkg]; !ok {
			t.Errorf("architecture contract requires %q to be in forbiddenByPackage; add it to keep the architecture test aligned with AGENTS.md", pkg)
		}
	}
}

// TestPkgImportsAreAllowlisted walks every Go file under each pkg/
// package and asserts every internal/... import is in that
// package's documented allowlist (see
// allowedInternalImportsPerPackage). The pkg/ packages are the
// external surface of the project; without this guard a future
// contributor can quietly re-export internal/ types to external
// tools without review. The allowlists mirror the current
// imports — a new internal/ import requires updating the
// allowlist in the same commit.
func TestPkgImportsAreAllowlisted(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("locate repo root: %v", err)
	}

	for pkg, allowed := range allowedInternalImportsPerPackage {
		pkgDir := filepath.Join(repoRoot, pkg)
		if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
			t.Fatalf("pkg package %q does not exist at %s", pkg, pkgDir)
		}

		fset := token.NewFileSet()
		err := filepath.WalkDir(pkgDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				name := d.Name()
				if name == "vendor" || name == "testdata" || (strings.HasPrefix(name, ".") && name != "." && name != "..") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}

			f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return fmt.Errorf("parse %s: %w", path, err)
			}

			relPath, _ := filepath.Rel(repoRoot, path)
			for _, imp := range f.Imports {
				importPath := strings.Trim(imp.Path.Value, `"`)
				if !strings.HasPrefix(importPath, "github.com/valueforvalue/DixieData/internal/") {
					continue
				}
				if !allowed[importPath] {
					pos := fset.Position(imp.Pos())
					t.Errorf("pkg import outside allowlist in %s\n  file: %s:%d\n  package: %q\n  forbidden import: %q\n  fix: add the import to allowedInternalImportsPerPackage[%q] if the new import is intentional, or move the code that needs it to a deeper package", pkg, relPath, pos.Line, pkg, importPath, pkg)
				}
			}
			return nil
		})
		if err != nil {
			t.Errorf("walk %s: %v", pkg, err)
		}
	}
}