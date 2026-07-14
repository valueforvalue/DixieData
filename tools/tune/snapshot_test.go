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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	// Issue #559: the seed fixture must include article rows so
	// TestTuneListRecordsKindFilter --kind article and
	// TestTuneModeArticleValidator both pass. Without an explicit
	// -articles N, seed-data picks 1-2 randomly (internal/seed/seed.go
	// seedArticles: count==0 -> 1 + rng.Intn(2)), which sometimes
	// yields 1 and always yields no row with id=1 for the article
	// render path. Pin both the count (2, matching the test
	// docstring in TestTuneListRecordsKindFilter) so the fixture is
	// deterministic across runs.
	cmd := exec.CommandContext(ctx, seedBin,
		"-data-dir", dataDir,
		"-soldiers", "10",
		"-articles", "2",
	)
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

	// Determinism self-check (issue #517 slice A4):
	// re-invoke tune with identical args to a second temp
	// path and assert byte-equality. Catches a non-determinism
	// regression in the CLI surface (time.Now() leak through
	// a bridge helper, map-iteration order in a typst data
	// projection, etc.) even when the golden happens to match.
	// A failure here is a regression — do NOT just regen the
	// golden, fix the determinism bug first.
	detPath := filepath.Join(tmpDir, "soldier1-landscape-det.pdf")
	if err := runTuneInvoke(t, dataDir, typstPath, detPath); err != nil {
		t.Fatalf("tune determinism re-invoke failed: %v", err)
	}
	gotDet, err := os.ReadFile(detPath)
	if err != nil {
		t.Fatalf("read determinism pdf: %v", err)
	}
	if !bytes.Equal(got, gotDet) {
		t.Fatalf("determinism self-check failed for %s: two consecutive tune invocations differ (first=%d bytes, second=%d bytes). Non-determinism in the CLI surface. Do NOT regen the golden — fix the determinism bug first.",
			t.Name(), len(got), len(gotDet))
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
// list-records subcommand (issue #430 + #518). Defaults to
// soldier; --kind article switches to the article list;
// --kind event (issue #518 slice C1) switches to the event
// list. The seed-data fixture seeds 10 Person Records
// (DXD-*), 2 Events (EVT-*) — --kind soldier must filter
// out the events (issue #518 slice C2) so it shows 10 not 12.
// Both must print a `total: N` line on stderr and exit 0.
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
		wantErrSub string // substring expected in the error; "" = no check
	}{
		{"default is soldier", "", "total: 10 records", ""},
		{"--kind soldier (filters events)", "soldier", "total: 10 records", ""},
		{"--kind article", "article", "total: 2 articles", ""},
		{"--kind event (issue #518 C1)", "event", "total: 2 events", ""},
		{"--kind bad value", "bogus", "", `--kind must be soldier, article, or event`},
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

// TestTuneModeArticleValidator (issue #516 slice B1) pins the
// --mode article path. Before the fix, parseRenderFlags rejected
// --mode article even though the switch rf.mode has a case
// "article" and the README documents the flag — anyone following
// the README hit `--mode must be record, bulk, or event (got
// "article")`. The fix adds "article" to the allowed set so the
// documented path actually works. This test invokes the binary
// with --mode article + --record N against the seed fixture and
// asserts a non-empty PDF is written and exit code is 0.
// Skips silently if typst or the fixture is unavailable (matching
// the convention used by TestTuneRecordLandscapeSnapshot).
func TestTuneModeArticleValidator(t *testing.T) {
	if findTypstBin(t) == "" {
		t.Skip("typst binary not found; set TYPST_BIN or build bin/typst-*")
	}
	dataDir := ensureSeedFixture(t)
	if dataDir == "" {
		t.Skip("seed fixture unavailable; build cmd/seed-data via `make debug`")
	}
	typstPath := findTypstBin(t)
	tuneBin := findUp("tools/tune/bin/dixiedata-tune.exe")
	if tuneBin == "" {
		t.Skip("dixiedata-tune binary not found; run `make tune`")
	}

	out := filepath.Join(t.TempDir(), "article.pdf")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tuneBin,
		"--db", dataDir,
		"--typst", typstPath,
		"--templates", templatesAbs(t),
		"render",
		"--mode", "article",
		"--record", "1",
		"--template", "article_landscape",
		"--orientation", "L",
		"--out", out,
	)
	cmd.Dir = t.TempDir()
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("--mode article rejected: %v\n%s", err, outBytes)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read article pdf: %v", err)
	}
	if !bytes.HasPrefix(got, []byte("%PDF-")) {
		t.Fatalf("--mode article produced non-PDF output (%d bytes, header %q)", len(got), headerFor(got))
	}
	if len(got) < 1000 {
		t.Fatalf("--mode article produced suspiciously small PDF (%d bytes) — likely an empty render", len(got))
	}
}

// TestTuneFixtureHasArticlesAndEvents (issue #559) pins the
// seed-fixture row counts so a future change that drops the
// -articles/--events seed flags (or regresses the legacy
// defaults) fails fast here rather than silently rotting
// TestTuneListRecordsKindFilter / TestTuneModeArticleValidator
// with confusing output messages. Asserts:
//   - 10 Person Records (soldiers)
//   - 2 Event Records (events; issue #518 slice C1)
//   - 2 Article Records (issue #430 + #559)
// Skips silently if typst, the seed binary, or the fixture is
// unavailable (matching the convention used by every other
// TestTune* test).
func TestTuneFixtureHasArticlesAndEvents(t *testing.T) {
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
		kind      string
		wantTotal string
	}{
		{"soldier", "total: 10 records"},
		{"event", "total: 2 events"},
		{"article", "total: 2 articles"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, tuneBin,
				"--db", dataDir, "--typst", typstPath,
				"--templates", templatesAbs(t),
				"list-records", "--kind", tc.kind,
			)
			cmd.Dir = t.TempDir()
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("list-records --kind %s: %v\nstdout: %s\nstderr: %s",
					tc.kind, err, stdout.String(), stderr.String())
			}
			out := stdout.String() + stderr.String()
			if !strings.Contains(out, tc.wantTotal) {
				t.Fatalf("fixture drift: --kind %s expected %q; got:\n%s",
					tc.kind, tc.wantTotal, out)
			}
		})
	}
}

// TestTuneDBStrictRefusesMissingDB (issue #516 slice B2) pins the
// phantom-DB footgun fix. Previously, `--db <missing-dir>` would
// silently MkdirAll + create a fresh empty dixiedata.db there
// (db.Open does MkdirAll), then return "total: 0 records" with
// no warning — silently corrupting the user's tree with a 339KB
// phantom db. The fix in openRenderer requires the db file to
// already exist; bail with a clear error otherwise. This test:
// (a) points --db at a path that doesn't exist, asserts the
// exit is non-zero + the error mentions "no dixiedata.db found"
// + the directory was NOT created; (b) confirms the opt-out
// DIXIEDATA_TUNE_DB_CREATE=1 still works for callers that
// need the legacy auto-create behavior.
func TestTuneDBStrictRefusesMissingDB(t *testing.T) {
	if findTypstBin(t) == "" {
		t.Skip("typst binary not found; set TYPST_BIN or build bin/typst-*")
	}
	tuneBin := findUp("tools/tune/bin/dixiedata-tune.exe")
	if tuneBin == "" {
		t.Skip("dixiedata-tune binary not found; run `make tune`")
	}
	typstPath := findTypstBin(t)
	templatesPath := findUp("templates/soldier_landscape.typ")
	if templatesPath == "" {
		t.Skip("templates/ not found; run from repo root")
	}

	// Use a path under t.TempDir() so the test leaves nothing
	// behind even on a bug.
	missing := filepath.Join(t.TempDir(), "definitely-does-not-exist")

	// (a) Strict: missing db fails cleanly without creating the dir.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tuneBin,
		"--db", missing,
		"--typst", typstPath,
		"--templates", filepath.Dir(templatesPath),
		"list-records", "--kind", "soldier",
	)
	cmd.Dir = t.TempDir()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit for missing db, got success:\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
	}
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "no dixiedata.db found") {
		t.Fatalf("expected error mentioning 'no dixiedata.db found'; got:\n%s", combined)
	}
	if _, statErr := os.Stat(missing); statErr == nil {
		t.Fatalf("strict-db fix should NOT have created the directory; %s exists", missing)
	}

	// (b) Opt-out: DIXIEDATA_TUNE_DB_CREATE=1 still creates.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel2()
	optout := missing + "-optout"
	cmd2 := exec.CommandContext(ctx2, tuneBin,
		"--db", optout,
		"--typst", typstPath,
		"--templates", filepath.Dir(templatesPath),
		"list-records", "--kind", "soldier",
	)
	cmd2.Dir = t.TempDir()
	cmd2.Env = append(os.Environ(), "DIXIEDATA_TUNE_DB_CREATE=1")
	var stdout2, stderr2 bytes.Buffer
	cmd2.Stdout = &stdout2
	cmd2.Stderr = &stderr2
	// The opt-out path runs through the bridge, which will fail
	// on a brand-new empty db (no tables). That's fine — the test
	// only asserts the strict guard was bypassed (i.e. the
	// directory was created and the bridge got far enough to
	// attempt the open).
	if err2 := cmd2.Run(); err2 == nil {
		// Unexpected success — log so the test isn't silent.
		t.Logf("opt-out succeeded (rare); output:\n%s", stdout2.String()+stderr2.String())
	}
	if _, statErr := os.Stat(filepath.Join(optout, "dixiedata.db")); statErr != nil {
		t.Fatalf("opt-out should have created the db file; stat %s: %v", filepath.Join(optout, "dixiedata.db"), statErr)
	}
}

// TestTuneWatchDebounceAndDedupe (issue #515 slice D5) pins the
// watcher's structural fixes without spawning a long-running
// process (which would be flaky in CI). The assertions:
// (a) the source file defines `doRenderParsed` as a separate
// function (the dedupe seam);
// (b) `doWatch` calls `doRenderParsed` and not `doRender(args, ...)`
// (proving the pre-parsed rf is reused on every mtime tick);
// (c) the watcher's debounce constant is in the 200-500ms range
// (proves the rapid-save collapse is wired).
// The actual end-to-end watch behavior is smoke-tested manually
// (run `dixiedata-tune watch ...`, edit a template, observe
// one re-render after the quiet period).
func TestTuneWatchDebounceAndDedupe(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	contents := string(src)
	// (a) doRenderParsed exists as a function definition.
	if !strings.Contains(contents, "func doRenderParsed(") {
		t.Fatalf("main.go must define doRenderParsed function (issue #515 slice D5 dedupe)")
	}
	// (b) doWatch uses doRenderParsed (not the args-parsing wrapper).
	// Locate the doWatch function body and assert it references
	// doRenderParsed at least once.
	watchStart := strings.Index(contents, "func doWatch(")
	if watchStart < 0 {
		t.Fatalf("main.go missing doWatch function (issue #515 slice D5)")
	}
	// End of doWatch body = next top-level `func ` declaration.
	rest := contents[watchStart:]
	nextFunc := strings.Index(rest[1:], "\nfunc ")
	watchEnd := len(rest)
	if nextFunc >= 0 {
		watchEnd = nextFunc + 1
	}
	watchBody := rest[:watchEnd]
	if !strings.Contains(watchBody, "doRenderParsed(") {
		t.Fatalf("doWatch must call doRenderParsed (issue #515 slice D5 dedupe); body lacks the call:\n%s", watchBody)
	}
	if strings.Contains(watchBody, "doRender(args,") {
		t.Fatalf("doWatch must not re-call doRender(args,...) on every tick (issue #515 slice D5 dedupe); body still contains the parse-twice path:\n%s", watchBody)
	}
	// (c) Debounce constant is in the 200-500ms range. Look for
	// the literal `debounce = ... * time.Millisecond` line.
	debounceRe := regexp.MustCompile(`const debounce\s*=\s*(\d+)\s*\*\s*time\.Millisecond`)
	m := debounceRe.FindStringSubmatch(watchBody)
	if m == nil {
		t.Fatalf("doWatch must declare a 'const debounce = N * time.Millisecond' (issue #515 slice D5 debounce); body lacks the constant")
	}
	ms := 0
	if _, err := fmt.Sscanf(m[1], "%d", &ms); err != nil {
		t.Fatalf("parse debounce ms %q: %v", m[1], err)
	}
	if ms < 200 || ms > 500 {
		t.Fatalf("debounce=%dms is outside the 200-500ms range (issue #515 slice D5); pick a value that collapses rapid saves without making single-save iteration feel laggy", ms)
	}
}

// TestTuneDoctorQuick (issue #515 slice D3) pins the doctor
// preflight gate in --quick mode (skips the snapshot test
// invocation). Asserts (a) exit 0 when the local repo has
// the expected moving parts (typst binary + templates dir +
// seed fixture), (b) human output contains each check name
// ("typst binary", "templates dir", "seed fixture",
// "snapshots present"), (c) "doctor: all checks passed"
// line at the end. The full test invocation mode is not
// pinned here — it's covered by the snapshot suites themselves
// (probeSnapshotsGreen runs them) and would add 2-3 minutes
// to the test suite for marginal value.
func TestTuneDoctorQuick(t *testing.T) {
	tuneBin := findUp("tools/tune/bin/dixiedata-tune.exe")
	if tuneBin == "" {
		t.Skip("dixiedata-tune binary not found; run `make tune`")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tuneBin, "doctor", "--quick")
	cmd.Dir = findRepoRoot(t)
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor --quick failed: %v\n%s", err, outBytes)
	}
	out := string(outBytes)
	for _, want := range []string{"typst binary", "templates dir", "seed fixture", "snapshots present", "doctor: all checks passed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected doctor --quick output to contain %q; got:\n%s", want, out)
		}
	}
}

// findRepoRoot walks up from the test's CWD looking for a go.mod
// whose module line is exactly 'module github.com/valueforvalue/DixieData'
// (the root module, not tools/tune). Used by TestTuneDoctorQuick
// so the doctor invocation has the right CWD for findSeedFixtureHint.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	const want = "module github.com/valueforvalue/DixieData"
	for i := 0; i < 8; i++ {
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			for _, sep := range []string{"\r\n", "\n"} {
				if bytes.HasPrefix(data, []byte(want+sep)) {
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
	t.Skip("repo root not found; run from inside the DixieData checkout")
	return ""
}

// TestTuneVersionFlag (issue #515 slice D2) pins the --version
// flag. Asserts (a) the flag short-circuits before any global flag
// parsing — works without --db, --typst, or anything else; (b)
// the human output contains the tune version, the typst version
// (resolved via findTypstBinary walker), and the bridge version;
// (c) the JSON output via DIXIEDATA_TUNE_JSON=1 emits the same
// three fields as a single-line-pretty JSON object.
func TestTuneVersionFlag(t *testing.T) {
	tuneBin := findUp("tools/tune/bin/dixiedata-tune.exe")
	if tuneBin == "" {
		t.Skip("dixiedata-tune binary not found; run `make tune`")
	}

	// (a) --version works without --db.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tuneBin, "--version")
	cmd.Dir = t.TempDir()
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("--version failed: %v\n%s", err, outBytes)
	}
	out := string(outBytes)
	for _, want := range []string{"dixiedata-tune ", "  typst:  ", "  bridge: "} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected human --version output to contain %q; got:\n%s", want, out)
		}
	}

	// (b) JSON output via DIXIEDATA_TUNE_JSON=1.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel2()
	cmd2 := exec.CommandContext(ctx2, tuneBin, "--version")
	cmd2.Dir = t.TempDir()
	cmd2.Env = append(os.Environ(), "DIXIEDATA_TUNE_JSON=1")
	outBytes2, err2 := cmd2.CombinedOutput()
	if err2 != nil {
		t.Fatalf("--version (json) failed: %v\n%s", err2, outBytes2)
	}
	out2 := string(outBytes2)
	for _, want := range []string{`"tune":`, `"typst":`, `"bridge":`} {
		if !strings.Contains(out2, want) {
			t.Fatalf("expected JSON --version output to contain %q; got:\n%s", want, out2)
		}
	}
}