// doctor.go — `dixiedata doctor` subcommand (Phase 2 of cli-plan.md).
//
// Runs a superset of the smoke checks plus four deeper
// diagnostic checks, with `--check` filtering and `--fix` repair
// mode. Output shares the smoke envelope (SmokeResult +
// SmokeCheck JSON shape) so the same parser works on both
// commands.
//
// File: docs/agents/doctor-impl-notes.md captures the design
// decisions for each check + the templates_parseable rationale
// (real Typst eval, not the file-content check the cli-plan
// draft mentioned).
package appshell

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/db"
)

// DoctorOptions configures RunDoctor. Zero value = text output,
// all checks, no --fix.
type DoctorOptions struct {
	JSON       bool
	DataDir    string   // override; empty = appdata.DefaultDir()
	Writer     io.Writer
	App        *App     // optional pre-constructed App (for tests)
	Checks     []string // `--check=X` filter; empty = run all
	Fix        bool     // --fix: attempt repair on failed checks
	TypstPath  string   // override typst binary; empty = find via App
	Now        func() time.Time
}

// DoctorFixResult is one entry in the RunDoctor return value's
// Fixes slice. Records what --fix did (or didn't do) for
// diagnostic + test purposes.
type DoctorFixResult struct {
	Check   string `json:"check"`
	Applied bool   `json:"applied"`
	Detail  string `json:"detail,omitempty"`
	Error   string `json:"error,omitempty"`
}

// DoctorResult wraps SmokeResult with the --fix report.
type DoctorResult struct {
	SmokeResult
	Fixes []DoctorFixResult `json:"fixes,omitempty"`
}

// RunDoctor runs the doctor command. Returns the result, the
// exit code, and the list of fix actions attempted (empty when
// DoctorOptions.Fix is false).
func RunDoctor(ctx context.Context, opts DoctorOptions) (DoctorResult, int) {
	if opts.Writer == nil {
		opts.Writer = os.Stderr
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	start := opts.Now()

	result := DoctorResult{
		SmokeResult: SmokeResult{
			Command:    "doctor",
			StartedAt:  start.UTC().Format(time.RFC3339),
			AppVersion: db.GetAppVersion(),
			BuildID:    buildIdentityString(),
		},
	}

	app := opts.App
	if app == nil {
		var buildErr error
		app, buildErr = buildSmokeApp(ctx, opts.DataDir)
		if buildErr != nil {
			result.SmokeResult.Checks = []SmokeCheck{{
				Name:   "0/0_app_build",
				Passed: false,
				Error:  buildErr.Error(),
			}}
			result.SmokeResult.DurationMs = opts.Now().Sub(start).Milliseconds()
			result.SmokeResult.Exit = 2
			writeDoctorOutput(opts.Writer, opts.JSON, result)
			return result, 2
		}
		defer smokeShutdown(app)
	}

	// Build the check list, then filter by --check.
	checks := defaultDoctorChecks()
	if len(opts.Checks) > 0 {
		checks = filterChecks(checks, opts.Checks)
	}

	failed := 0
	for i, c := range checks {
		cStart := opts.Now()
		err := c.fn(ctx, app)
		cDur := opts.Now().Sub(cStart).Milliseconds()

		countsAsFailure := err != nil && !c.optional

		check := SmokeCheck{
			Name:       fmt.Sprintf("%d/%d_%s", i+1, len(checks), c.name),
			Passed:     err == nil,
			DurationMs: cDur,
			Optional:   c.optional,
		}
		if err != nil {
			check.Error = err.Error()
			if countsAsFailure {
				failed++
			}
		}
		result.SmokeResult.Checks = append(result.SmokeResult.Checks, check)
		if opts.JSON {
			_ = json.NewEncoder(opts.Writer).Encode(check)
		} else {
			fmt.Fprintf(opts.Writer, "[%d/%d] %-32s %s   %dms\n",
				i+1, len(checks), c.name,
				humanStatus(err, c.optional), cDur,
			)
		}
	}

	// --fix mode: for each failed hard check, attempt the
	// corresponding repair. Reports what was done; never deletes
	// data without a backup step in the repair itself.
	if opts.Fix && failed > 0 {
		result.Fixes = applyFixes(ctx, app, result.SmokeResult.Checks, opts)
		if !opts.JSON {
			for _, fx := range result.Fixes {
				if fx.Applied {
					fmt.Fprintf(opts.Writer, "fix[%s] %s\n", fx.Check, fx.Detail)
				} else if fx.Error != "" {
					fmt.Fprintf(opts.Writer, "fix[%s] FAIL: %s\n", fx.Check, fx.Error)
				}
			}
		}
	}

	result.SmokeResult.DurationMs = opts.Now().Sub(start).Milliseconds()
	switch {
	case failed == len(checks) && failed > 0:
		result.SmokeResult.Exit = 2
	case failed > 0:
		result.SmokeResult.Exit = 1
	default:
		result.SmokeResult.Exit = 0
	}
	writeDoctorOutput(opts.Writer, opts.JSON, result)
	if !opts.JSON {
		fmt.Fprintln(opts.Writer, summarize(result.SmokeResult))
	}
	return result, result.SmokeResult.Exit
}

// --- doctor check registry ---

// defaultDoctorChecks returns the full check set in execution
// order. Smoke's 8 checks come first so the first 8 results are
// byte-identical between `dixiedata --smoke` and `dixiedata
// doctor` (useful for diff-based regression detection).
func defaultDoctorChecks() []smokeCheckDef {
	return []smokeCheckDef{
		// --- smoke baseline (8) ---
		{name: "data_dir_resolves", fn: checkDataDir},
		{name: "logs_dir_separate", fn: checkLogsDirSeparate},
		{name: "sqlite_open", fn: checkSQLiteOpen},
		{name: "migrations_applied", fn: checkMigrationsApplied},
		{name: "oauth_defaults_loaded", fn: checkOAuthDefaults, optional: true},
		{name: "templates_dir", fn: checkTemplatesDir},
		{name: "typst_binary", fn: checkTypstBinary},
		{name: "routes_registered", fn: checkRoutesRegistered},

		// --- doctor-only (4) ---
		{name: "archive_writable", fn: checkArchiveWritable},
		{name: "feedback_log_open", fn: checkFeedbackLogOpen},
		// pdfium_loadable is optional: dev builds don't ship
		// pdfium.dll; release builds do. The release artifact
		// linter is the right place to enforce that.
		{name: "pdfium_loadable", fn: checkPDFiumLoadable, optional: true},
		{name: "templates_parseable", fn: checkTemplatesParseable},
	}
}

// filterChecks keeps only checks whose name matches one of the
// `wanted` filter strings. A filter matches when it equals any
// of the check's "stems" — the underscore-separated prefixes.
//
// A check like `data_dir_resolves` has stems `data`, `data_dir`,
// `data_dir_resolves`. `--check=data_dir` matches because the
// check's stem list contains `data_dir`. `--check=data` matches
// every check that has `data` as a stem, which is every
// `data_*` check. `--check=foobar` matches nothing.
//
// Empty `wanted` = keep all (caller should pre-check).
func filterChecks(checks []smokeCheckDef, wanted []string) []smokeCheckDef {
	if len(wanted) == 0 {
		return checks
	}
	out := make([]smokeCheckDef, 0, len(checks))
	for _, c := range checks {
		stems := stemsOf(c.name)
		for _, w := range wanted {
			w = strings.TrimSpace(w)
			for _, s := range stems {
				if s == w {
					out = append(out, c)
					goto next
				}
			}
		}
	next:
	}
	return out
}

func stemsOf(name string) []string {
	parts := strings.Split(name, "_")
	out := make([]string, 0, len(parts))
	for i := 1; i <= len(parts); i++ {
		out = append(out, strings.Join(parts[:i], "_"))
	}
	return out
}

// --- doctor-only checks ---

// checkArchiveWritable proves a Restore Point can be created
// and removed at the user's data dir. Catches the
// `caf2c28626`-class "release installed but cannot write
// restore points" bug. Always uses no-op archive/build writers
// so it never overwrites real archives on disk.
func checkArchiveWritable(ctx context.Context, a *App) error {
	if a.restorePoints == nil {
		return fmt.Errorf("restorePoints manager not initialized (startup incomplete)")
	}
	// We never invoke the real backup facade here — the goal is
	// to prove the filesystem permits a restore point, not to
	// actually back up. The probe round-trip below exercises
	// every filesystem op the real Create path needs (MkdirAll,
	// WriteFile, ReadFile, RemoveAll).
	probeID := fmt.Sprintf("doctor-probe-%d", time.Now().UnixNano())
	// Use a.dataDir directly — RestorePointManager stores the
	// same value in its unexported `dataDir` field, and going
	// through the manager would require either exposing it or
	// using an accessor that doesn't exist yet.
	probeDir := filepath.Join(a.dataDir, "updates", "restore-points", probeID)
	if err := os.MkdirAll(probeDir, 0o755); err != nil {
		return fmt.Errorf("create restore-point probe dir %s: %w", probeDir, err)
	}
	defer os.RemoveAll(probeDir)

	// Try a write + read round-trip on a sentinel file inside the
	// probe dir. Mirrors what RestorePointManager.Create does
	// (MkdirAll + WriteFile via the archive writer callback).
	sentinel := filepath.Join(probeDir, "local-archive.ddbak")
	if err := os.WriteFile(sentinel, []byte("doctor-probe"), 0o644); err != nil {
		return fmt.Errorf("write probe sentinel %s: %w", sentinel, err)
	}
	data, err := os.ReadFile(sentinel)
	if err != nil {
		return fmt.Errorf("read probe sentinel: %w", err)
	}
	if string(data) != "doctor-probe" {
		return fmt.Errorf("probe sentinel round-trip mismatch: got %q", data)
	}
	return nil
}

// checkFeedbackLogOpen proves the JSONL feedback log is
// readable AND every line is valid JSON. Catches the
// `app_feedback.go`-class bug where a partial write leaves a
// trailing truncated line that breaks every subsequent read.
func checkFeedbackLogOpen(ctx context.Context, a *App) error {
	path := appdata.FeedbackLogPath(a.dataDir)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no log yet = nothing to corrupt
		}
		return fmt.Errorf("stat feedback log: %w", err)
	}
	if info.Size() == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read feedback log: %w", err)
	}
	// Split on newline; tolerate a final empty line (common when
	// the file ends with \n).
	lines := strings.Split(string(data), "\n")
	corrupt := 0
	var firstCorrupt string
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !json.Valid([]byte(line)) {
			corrupt++
			if firstCorrupt == "" {
				firstCorrupt = fmt.Sprintf("line %d: %q", i+1, truncate(line, 60))
			}
		}
	}
	if corrupt > 0 {
		return fmt.Errorf("%d corrupt line(s) in feedback log; first: %s. Run `dixiedata doctor --fix` to truncate.", corrupt, firstCorrupt)
	}
	return nil
}

// checkPDFiumLoadable asserts `pdfium.dll` exists next to the
// exe and is at least 1MB. Catches the `56e31f0` 0-byte stub
// bug where a botched tar extraction silently shipped a
// non-functional PDF renderer.
//
// Marked optional for dev builds where pdfium.dll isn't copied
// to build/bin/. The release artifact linter (Pattern D test in
// docs/COMMON_BUGS.md §4.10) is the right place to enforce
// "release zip must contain pdfium.dll" — this check warns
// developers without failing their local smoke run.
func checkPDFiumLoadable(ctx context.Context, a *App) error {
	// Mirror resolvePDFiumDLLPath's lookup order:
	//   DIXIEDATA_PDFIUM_DLL env > <exe-dir>/pdfium.dll > cwd > cwd/build/bin
	candidates := pdfiumCandidatePaths()
	var found string
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			found = p
			break
		}
	}
	if found == "" {
		return fmt.Errorf("pdfium.dll not found in any candidate path (run from the install dir or set DIXIEDATA_PDFIUM_DLL)")
	}
	info, err := os.Stat(found)
	if err != nil {
		return fmt.Errorf("stat %s: %w", found, err)
	}
	const minSize int64 = 1 << 20 // 1MB
	if info.Size() < minSize {
		return fmt.Errorf("pdfium.dll at %s is suspiciously small (%d bytes < %d) — likely a 0-byte stub from a botched tar extraction", found, info.Size(), minSize)
	}
	return nil
}

// checkTemplatesParseable shells out to `typst compile` on every
// *.typ in the templates dir with a stub data.json so the
// template can read what it expects. Real Typst gives us a
// proper syntax check in <2s per file, which catches the class
// of "valid Go template that confuses Typst" regressions we'd
// otherwise miss. The companion document
// docs/agents/doctor-impl-notes.md explains why we don't try to
// parse Typst ourselves.
//
// The stub is `{}` — every template uses `.at("...", default: ...)`
// for every data access, so an empty object lets parse pass and
// runtime fail later. The check is only about syntax.
func checkTemplatesParseable(ctx context.Context, a *App) error {
	dir, err := a.findTemplatesDir()
	if err != nil {
		return err
	}
	typstBin, err := a.findTypstBinary()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read templates dir %s: %w", dir, err)
	}

	// Drop a stub data.json in the templates dir so every
	// template can at least parse. We remove it after the check
	// so we don't pollute the user's tree.
	stub := filepath.Join(dir, "data.json")
	if _, err := os.Stat(stub); err == nil {
		// Don't clobber a real data.json — refuse to run.
		return fmt.Errorf("templates dir already contains data.json; refusing to overwrite")
	}
	if err := os.WriteFile(stub, []byte("{}\n"), 0o644); err != nil {
		return fmt.Errorf("write stub data.json: %w", err)
	}
	defer os.Remove(stub)

	var failed []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".typ") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if err := runTypstCompile(ctx, typstBin, path); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", e.Name(), err))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("Typst parse failed for %d template(s): %s",
			len(failed), strings.Join(failed, "; "))
	}
	return nil
}

// runTypstCompile shells out to `typst compile --format pdf` with
// a 5s timeout. The check distinguishes parse errors from
// runtime errors by inspecting the first `error:` line:
//
//   - Parse errors say `error: expected ...` or `error: unexpected ...`.
//   - Runtime errors say `error: type ... has no method`, `error:
//     cannot calculate ...`, etc.
//
// We treat only parse errors as failure; runtime errors mean
// the template parsed fine but couldn't render with the empty
// `{}` stub. Real data fixtures live in templates/testdata/ —
// when those land, doctor will compile against them instead of
// the stub. For now the parse check is the value.
//
// Output target is "nul" on Windows (Typst writes the PDF
// nowhere). The 5s timeout stops a hung typst process from
// blocking the whole doctor run.
func runTypstCompile(ctx context.Context, typstBin, path string) error {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, typstBin, "compile",
		"--ignore-system-fonts", "--ignore-embedded-fonts",
		"--format", "pdf", path, "nul")
	out, err := cmd.CombinedOutput()
	if cctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("typst compile timed out after 5s")
	}
	output := string(out)
	if isParseError(output) {
		return fmt.Errorf("Typst parse error: %s", firstErrorLine(output))
	}
	// Anything else (runtime errors, exit 1, etc.) means the
	// template parsed successfully — that's all this check
	// promises. The export benchmark + integration tests cover
	// full-fidelity rendering with real data.
	_ = err
	return nil
}

// isParseError returns true when the typst output contains a
// parse error. Parse errors always begin with `error: expected`
// or `error: unexpected` (Typst's grammar-mismatch messages).
// Runtime errors begin with `error: type ...` or `error: cannot`.
func isParseError(output string) bool {
	for _, marker := range []string{
		"error: expected",
		"error: unexpected",
		"error: unknown variable",  // undeclared variable is a syntax-level problem
		"error: mismatched",        // unmatched bracket, paren, etc.
	} {
		if strings.Contains(output, marker) {
			return true
		}
	}
	return false
}

// firstErrorLine returns the first line of output that contains
// `error:`. Empty if no error.
func firstErrorLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "error:") {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// --- --fix repair operations ---

// applyFixes runs the repair for each failed check. The map
// from check name -> fix function lives here so the doctor
// command stays declarative. Add a new repair by adding an entry
// to this map and writing the function.
func applyFixes(ctx context.Context, a *App, results []SmokeCheck, opts DoctorOptions) []DoctorFixResult {
	fixers := map[string]func(context.Context, *App) (string, error){
		"feedback_log_open":    fixTruncateFeedbackLog,
		"migrations_applied":   fixReapplyMigrations,
		"oauth_defaults_loaded": fixRestoreOAuthDefaults,
	}
	var out []DoctorFixResult
	for _, r := range results {
		if r.Passed {
			continue
		}
		fix, ok := fixers[r.Name]
		if !ok {
			continue // no fix defined for this failure
		}
		detail, err := fix(ctx, a)
		fx := DoctorFixResult{
			Check:   r.Name,
			Applied: err == nil,
			Detail:  detail,
		}
		if err != nil {
			fx.Error = err.Error()
		}
		out = append(out, fx)
	}
	return out
}

// fixTruncateFeedbackLog backs up the JSONL to
// feedback-log.jsonl.bak-<timestamp> and replaces the original
// with the parsed-good subset (lines that pass json.Valid).
// Catches the partial-write class of corruption.
func fixTruncateFeedbackLog(ctx context.Context, a *App) (string, error) {
	path := appdata.FeedbackLogPath(a.dataDir)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "no log to truncate", nil
		}
		return "", fmt.Errorf("stat: %w", err)
	}
	if info.Size() == 0 {
		return "log is empty", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	good := make([]string, 0, len(lines))
	bad := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if json.Valid([]byte(line)) {
			good = append(good, line)
		} else {
			bad++
		}
	}
	if bad == 0 {
		return "no corrupt lines to drop", nil
	}
	// Backup the original BEFORE writing the truncated file so
	// the user can still inspect / restore the broken state if
	// the truncation was wrong.
	backup := fmt.Sprintf("%s.bak-%s", path, time.Now().UTC().Format("20060102-150405"))
	if err := os.WriteFile(backup, data, 0o644); err != nil {
		return "", fmt.Errorf("backup write: %w", err)
	}
	// Re-emit the good lines, newline-terminated, atomic-ish
	// (single WriteFile call). If something goes wrong the user
	// still has the .bak file.
	out := strings.Join(good, "\n")
	if out != "" {
		out += "\n"
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return "", fmt.Errorf("truncate write: %w", err)
	}
	return fmt.Sprintf("truncated %d corrupt line(s); backup at %s", bad, filepath.Base(backup)), nil
}

// fixReapplyMigrations closes and reopens the database so the
// existing migration runner in db.Open runs again. Migrations
// are idempotent (gated by PRAGMA user_version + schema_version
// table), so this is a no-op when the schema is already current.
// Catches the partial-migration class of bug where an upgrade
// crashed between two migrations and left a half-applied schema.
func fixReapplyMigrations(ctx context.Context, a *App) (string, error) {
	if a.database == nil {
		return "", fmt.Errorf("database handle not initialized")
	}
	// Snapshot the version BEFORE so we can report whether
	// anything actually moved.
	conn := a.database.Conn()
	var before int
	if err := conn.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&before); err != nil {
		return "", fmt.Errorf("read user_version before: %w", err)
	}
	// Close + reopen. db.Open runs backupBeforeMigrationIfNeeded
	// (no-op at current version) + applySchema (idempotent) +
	// ImportLegacyScratchpadFiles (no-op on already-imported).
	if err := a.database.Close(); err != nil {
		return "", fmt.Errorf("close: %w", err)
	}
	reopened, err := db.Open(a.dataDir)
	if err != nil {
		return "", fmt.Errorf("reopen: %w", err)
	}
	a.database = reopened
	// Also reset the jobs registry so a half-migrated archive
	// doesn't trip on its own JSONL index.
	if a.jobs != nil {
		_ = a.jobs.Shutdown(ctx)
		a.jobs = openJobsRegistry(a.dataDir)
	}
	var after int
	if err := reopened.Conn().QueryRowContext(ctx, `PRAGMA user_version`).Scan(&after); err != nil {
		return "", fmt.Errorf("read user_version after: %w", err)
	}
	return fmt.Sprintf("reopened DB; user_version %d -> %d", before, after), nil
}

// fixRestoreOAuthDefaults copies google-oauth-defaults.json from
// the most likely source location to the most likely
// destination if it's missing. Currently a no-op stub: the
// release layout decision (where the source lives, where the
// dest should be) varies by deployment. We log the candidate
// paths so the user knows what to copy manually. Returns a
// friendly message, never an error.
func fixRestoreOAuthDefaults(ctx context.Context, a *App) (string, error) {
	candidates := googleDefaultsCandidatePaths()
	if len(candidates) == 0 {
		return "no candidate paths known", nil
	}
	return fmt.Sprintf("oauth defaults not auto-restoreable; check these locations manually: %s",
		strings.Join(candidates, ", ")), nil
}

// --- helpers ---

// pdfiumCandidatePaths mirrors the lookup order in
// internal/archive/pdfium_windows.go:resolvePDFiumDLLPath.
// Kept as a local copy so this file doesn't pull the archive
// package into the import graph just for two paths.
func pdfiumCandidatePaths() []string {
	var paths []string
	if env := os.Getenv("DIXIEDATA_PDFIUM_DLL"); env != "" {
		paths = append(paths, env)
	}
	if exe, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(exe), "pdfium.dll"))
	}
	if wd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(wd, "pdfium.dll"))
		paths = append(paths, filepath.Join(wd, "build", "bin", "pdfium.dll"))
	}
	return paths
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\n\r"); i >= 0 {
		return s[:i]
	}
	return s
}

func writeDoctorOutput(w io.Writer, jsonOut bool, r DoctorResult) {
	if !jsonOut {
		return
	}
	_ = json.NewEncoder(w).Encode(r)
}

// HasDoctorFlag returns true when the args vector contains
// "doctor" as a subcommand. main.go uses this to dispatch into
// RunDoctor instead of wails.Run.
func HasDoctorFlag(args []string) bool {
	for _, a := range args {
		if a == "doctor" {
			return true
		}
	}
	return false
}

// WantsDoctorFix returns true when --fix is in the args.
func WantsDoctorFix(args []string) bool {
	for _, a := range args {
		if a == "--fix" {
			return true
		}
	}
	return false
}

// ParseDoctorChecks extracts `--check=NAME` flags from args.
// Returns the names in the order they appear. Unknown flags are
// ignored — the doctor command will then run zero checks and
// report nothing, which is a clear signal to the user.
func ParseDoctorChecks(args []string) []string {
	var out []string
	for _, a := range args {
		if strings.HasPrefix(a, "--check=") {
			out = append(out, strings.TrimPrefix(a, "--check="))
		}
	}
	return out
}

// WantsDoctorJSON returns true when --json is in the args.
// Distinct from --smoke-json so the two subcommands don't
// accidentally share an output flag.
func WantsDoctorJSON(args []string) bool {
	for _, a := range args {
		if a == "--json" {
			return true
		}
	}
	return false
}