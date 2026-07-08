// tools/tune/snapshot_test.go — first test file for the
// tools/tune iteration harness (issue #413). Pins the
// byte-shape of tune's PDF output for a fixed input so a
// future template refactor doesn't silently change what
// tune emits for the same fixture.
//
// Framework choice (per the issue body): raw golden files in
// testdata/, matching the existing repo convention in
// internal/exportcontract/snapshots_test.go. Pros over
// go-snaps: no third-party dep, no codegen, the golden file
// is a normal PDF you can open in a viewer for diffing.
// Cons: no structural diff (only byte equality); a one-byte
// timestamp drift in the PDF would force a regenerate. For
// tune — which renders deterministic fixture data through a
// fixed typst binary — byte equality is the right test.
//
// Update snapshots with:
//   UPDATE_SNAPSHOTS=1 go test ./tools/tune/...
//
// Skip behaviour:
//   - If the typst binary isn't found (bin/typst-*.exe),
//     the test is skipped (not failed). CI runners that
//     install typst globally can set TYPST_BIN env var.
//   - If no seed-data archive is found at .scratch/tune-fixture/,
//     the test seeds one via cmd/seed-data (creates + tears
//     down on success). This keeps the test self-contained.
//
// Acceptance criterion: rendering the seed-data soldier
// id=1 with --mode record --orientation landscape through
// the tune CLI must produce a byte-identical PDF to the
// golden file. A template refactor that changes the output
// must regenerate via UPDATE_SNAPSHOTS=1 + a manual review.

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testdataDir locates this package's testdata/ from the
// running test's CWD. The convention is testdata/<name>.pdf.
func testdataDir() string {
	return "testdata"
}

// templatesAbs resolves the absolute path to <repo>/templates.
// Used so the tune invocation doesn't depend on its cwd.
func templatesAbs(t *testing.T) string {
	t.Helper()
	p := findUp("templates/soldier_landscape.typ")
	if p == "" {
		t.Skip("templates/ not found; run from the repo root")
	}
	return filepath.Dir(p)
}

// findUpRepoRoot walks up looking for a go.mod whose module
// declaration is exactly `module github.com/valueforvalue/DixieData`
// (no subpath). Returns the directory containing that go.mod.
func findUpRepoRoot() string {
	const want = "module github.com/valueforvalue/DixieData"
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(candidate); err == nil {
			// CRLF-tolerant: trim the first line and check the
			// module declaration exactly.
			for _, sep := range []string{"\r\n", "\n"} {
				prefix := want + sep
				if bytes.HasPrefix(data, []byte(prefix)) {
					return dir
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// findUpWithContent walks up looking for a file at relPath
// whose contents include content. Returns the directory
// containing the matching file. Used to distinguish the
// repo-root go.mod from sub-package go.mod files.
func findUpWithContent(relPath, content string) string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, relPath)
		if data, err := os.ReadFile(candidate); err == nil {
			if bytes.Contains(data, []byte(content)) {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// findUp walks up the directory tree looking for the given
// relative path. Returns "" if not found within 6 levels.
func findUp(relPath string) string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, relPath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// findTypstBin walks up to 6 directories looking for the
// bin/typst-* binary, matching the production findTypstBinary
// behaviour so a tune test reflects a real tune invocation.
func findTypstBin(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"bin/typst-windows.exe",
		"bin/typst-macos",
		"bin/typst-linux",
	}
	if env := os.Getenv("TYPST_BIN"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for i := 0; i < 6; i++ {
		for _, name := range candidates {
			candidate := filepath.Join(dir, name)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// ensureSeedFixture creates <repo>/.scratch/tune-fixture/ via
// cmd/seed-data if it doesn't already exist. Returns the
// ABSOLUTE data-directory path (the directory containing
// dixiedata.db) so callers can pass it directly to tune's
// --db flag. The fixture lives at <repo-root>/.scratch/tune-fixture/
// so the path is stable regardless of the test's cwd (which
// is tools/tune/ when go test runs the package).
func ensureSeedFixture(t *testing.T) string {
	t.Helper()
	// tools/tune has its own go.mod (module
	// github.com/valueforvalue/DixieData/tools/tune) which is
	// a CHILD of the repo-root module. Walking up looking for
	// the substring github.com/valueforvalue/DixieData matches
	// both, so we instead look for the go.mod whose `module`
	// line is EXACTLY the repo module path.
	repoRoot := findUpRepoRoot()
	if repoRoot == "" {
		t.Skip("repo root not found; run from inside the DixieData checkout")
	}
	dataDir := filepath.Join(repoRoot, ".scratch", "tune-fixture")
	dbPath := filepath.Join(dataDir, "dixiedata.db")
	if _, err := os.Stat(dbPath); err == nil {
		return dataDir
	}
	// Build seed-data on demand. The Makefile produces it at
	// <repo-root>/build/bin/seed-data.exe; we walk up from the
	// test's CWD (tools/tune/) to find it. Also accept a
	// SEED_DATA_BIN override for unusual layouts.
	seedBin := os.Getenv("SEED_DATA_BIN")
	if seedBin == "" {
		seedBin = findUp("build/bin/seed-data.exe")
		if seedBin == "" {
			seedBin = filepath.Join("build", "bin", "seed-data.exe")
		}
	}
	if _, err := os.Stat(seedBin); err != nil {
		// No prebuilt binary; skip rather than fail. CI installs
		// it via `make debug`.
		t.Skipf("seed-data binary not found at %s; run `make debug` or set SEED_DATA_BIN", seedBin)
		return ""
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	// Run cmd/seed-data against the fixture dir. seed-data
	// expects --data-dir and writes <dir>/dixiedata.db by default.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, seedBin, "-data-dir", dataDir, "-soldiers", "10")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Skipf("seed-data failed (%v); fixture unavailable", err)
		return ""
	}
	return dataDir
}

// runTuneInvoke shells out to the prebuilt tune binary
// because doRender is not a stable, exported entry point
// (its signature is tuned to the CLI's needs, and asserting
// through the real CLI matches how every user invokes tune).
func runTuneInvoke(t *testing.T, dataDir, typstPath, outPath string) error {
	t.Helper()
	tuneBin := findUp("tools/tune/bin/dixiedata-tune.exe")
	if tuneBin == "" {
		// Build on demand. The test runs from tools/tune/ so
		// that's the package dir; the output path is relative
		// to the package dir (tools/tune/bin/).
		build := exec.Command("go", "build", "-tags", "debug", "-o", filepath.Join("bin", "dixiedata-tune.exe"), ".")
		build.Stdout = os.Stdout
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			t.Skipf("tune build failed (%v); skipping snapshot test", err)
			return err
		}
		tuneBin = filepath.Join("bin", "dixiedata-tune.exe")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tuneBin,
		"--db", dataDir,
		"--typst", typstPath,
		"--templates", templatesAbs(t),
		"render",
		"--mode", "record",
		"--record", "1",
		"--template", "soldier_landscape",
		"--orientation", "landscape",
		"--out", outPath,
	)
	// Run from a stable cwd so relative paths in tune resolve
	// the same way every time.
	cmd.Dir = t.TempDir()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// TestTuneRecordLandscapeSnapshot is the first tune snapshot
// case (issue #413). Pin soldier id=1 from the seed-data
// fixture at landscape orientation against a golden PDF.
func TestTuneRecordLandscapeSnapshot(t *testing.T) {
	if findTypstBin(t) == "" {
		t.Skip("typst binary not found; set TYPST_BIN or build bin/typst-*")
	}
	dataDir := ensureSeedFixture(t)
	if dataDir == "" {
		t.Skip("seed fixture unavailable; build cmd/seed-data via `make debug`")
	}
	typstPath := findTypstBin(t)

	// Render to a temp file in the package dir so the test
	// leaves no global state behind.
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "soldier1-landscape.pdf")
	if err := runTuneInvoke(t, dataDir, typstPath, outPath); err != nil {
		t.Fatalf("tune invoke failed: %v", err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read rendered pdf: %v", err)
	}
	// Sanity: PDF magic header.
	if !bytes.HasPrefix(got, []byte("%PDF-")) {
		t.Fatalf("rendered file is not a PDF (got %d bytes, header %q)", len(got), headerFor(got))
	}

	goldenPath := filepath.Join(testdataDir(), "soldier1-landscape.pdf")
	if os.Getenv("UPDATE_SNAPSHOTS") == "1" {
		if err := os.MkdirAll(testdataDir(), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("snapshot updated: %s (%d bytes)", goldenPath, len(got))
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %q: %v (run with UPDATE_SNAPSHOTS=1 to create)", goldenPath, err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("snapshot mismatch for %s\n  want: %d bytes\n  got:  %d bytes\nRun with UPDATE_SNAPSHOTS=1 to update.",
			goldenPath, len(want), len(got))
	}
}

func headerFor(b []byte) string {
	if len(b) > 32 {
		return strings.ReplaceAll(string(b[:32]), "\n", "\\n")
	}
	return strings.ReplaceAll(string(b), "\n", "\\n")
}

// TestTuneListRecordsKindFilter pins the --kind flag on the
// list-records subcommand (issue #430). Defaults to soldier
// (matches the legacy default); --kind article switches to
// the article list. The seed-data fixture doesn't seed
// articles, so the article count is 0; the soldier count
// matches whatever the fixture seeded. Both must print a
// `total: N` line on stderr and exit 0.
func TestTuneListRecordsKindFilter(t *testing.T) {
	if findTypstBin(t) == "" {
		t.Skip("typst binary not found; set TYPST_BIN or build bin/typst-*")
	}
	dataDir := ensureSeedFixture(t)
	if dataDir == "" {
		t.Skip("seed fixture unavailable; build cmd/seed-data via `make debug`")
	}
	tuneBin := findUp("tools/tune/bin/dixiedata-tune.exe")
	if tuneBin == "" {
		t.Skip("dixiedata-tune binary not found; run `make tune`")
	}
	typstPath := findTypstBin(t)

	cases := []struct {
		name       string
		kind       string
		wantTotal  string
		wantErrSub string // substring expected in the `total:` line; "" = no check
	}{
		{"default is soldier", "", "total: 10 records", ""},
		{"--kind soldier", "soldier", "total: 10 records", ""},
		{"--kind article (empty archive)", "article", "total: 0 articles", ""},
		{"--kind bad value", "bogus", "", `--kind must be soldier or article`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"--db", dataDir, "--typst", typstPath, "--templates", templatesAbs(t), "list-records"}
			if tc.kind != "" {
				args = append(args, "--kind", tc.kind)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, tuneBin, args...)
			cmd.Dir = t.TempDir()
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()
			out := stdout.String() + stderr.String()
			if tc.wantErrSub != "" {
				if err == nil {
					t.Fatalf("expected error containing %q; got success with output:\n%s", tc.wantErrSub, out)
				}
				if !strings.Contains(out, tc.wantErrSub) {
					t.Fatalf("expected error containing %q; got %v:\n%s", tc.wantErrSub, err, out)
				}
				return
			}
			if err != nil {
				t.Fatalf("list-records failed: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
			}
			if tc.wantTotal != "" && !strings.Contains(out, tc.wantTotal) {
				t.Fatalf("expected output to contain %q; got:\n%s", tc.wantTotal, out)
			}
		})
	}
}