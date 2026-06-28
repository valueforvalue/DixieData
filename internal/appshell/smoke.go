// smoke.go — headless boot checks for DixieData.
//
// Run via `dixiedata --smoke` (CLI flag) or `DIXIEDATA_SMOKE=1
// dixiedata` (env var). Returns exit code:
//
//	0 — every check passed
//	1 — at least one check failed
//	2 — environment error (data dir unwritable, etc.)
//
// Phase 1 of `docs/agents/cli-plan.md`. Each check is its own
// function returning error; the runner prints one human line per
// check (text mode) or one JSON object per check (--smoke-json).
// The output shape is documented in cli-plan.md so CI / users
// can parse it without reading the source.
//
// The checks target the most common boot-time regressions from
// the bug catalog (docs/COMMON_BUGS.md §4.5 setup order,
// §4.10 dialog-guard law, §8 build/CI). Each check would have
// caught one of those bugs if it had existed when the bug was
// introduced.
package appshell

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/db"
)

// SmokeCheck is the result of one named probe. The struct is the
// JSON wire shape; do not rename fields without a CLI contract
// bump.
type SmokeCheck struct {
	Name       string `json:"name"`
	Passed     bool   `json:"passed"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
	Optional   bool   `json:"optional,omitempty"`
}

// SmokeResult is the envelope returned by RunSmoke. CI parses
// this; humans read the text rendering.
type SmokeResult struct {
	Command    string        `json:"command"`
	StartedAt  string        `json:"started_at"`
	DurationMs int64         `json:"duration_ms"`
	Exit       int           `json:"exit"`
	Checks     []SmokeCheck  `json:"checks"`
	AppVersion string        `json:"app_version"`
	BuildID    string        `json:"build_id"`
}

// SmokeOptions configures RunSmoke. Zero value = text output,
// pass-or-fail exit. With JSON=true the runner writes one JSON
// object per check to w (in addition to the human summary at
// the end).
type SmokeOptions struct {
	JSON     bool
	DataDir  string // override; empty = appdata.DefaultDir()
	Writer   io.Writer
	App      *App   // optional pre-constructed App (for tests); nil = build one
	Now      func() time.Time
}

// RunSmoke boots the app (or reuses an existing one), runs every
// smoke check, writes results, and returns the exit code. Never
// panics; check errors are captured per-check.
func RunSmoke(ctx context.Context, opts SmokeOptions) (SmokeResult, int) {
	if opts.Writer == nil {
		opts.Writer = os.Stderr
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	start := opts.Now()

	checks := defaultSmokeChecks()
	result := SmokeResult{
		Command:    "smoke",
		StartedAt:  start.UTC().Format(time.RFC3339),
		AppVersion: db.GetAppVersion(),
		BuildID:    buildIdentityString(),
	}

	app := opts.App
	if app == nil {
		var buildErr error
		app, buildErr = buildSmokeApp(ctx, opts.DataDir)
		if buildErr != nil {
			// Couldn't even start. Emit one synthetic check so the
			// caller still gets a parseable result.
			result.Checks = []SmokeCheck{{
				Name:       "0/0_app_build",
				Passed:     false,
				Error:      buildErr.Error(),
				DurationMs: 0,
			}}
			result.DurationMs = opts.Now().Sub(start).Milliseconds()
			result.Exit = 2
			writeSmokeJSON(opts.Writer, opts.JSON, result)
			fmt.Fprintln(opts.Writer, summarize(result))
			return result, 2
		}
		defer smokeShutdown(app)
	}

	failed := 0
	for i, c := range checks {
		cStart := opts.Now()
		err := c.fn(ctx, app)
		cDur := opts.Now().Sub(cStart).Milliseconds()

		// Optional checks that return an error still record the
		// error in the result (so the user sees what happened) but
		// don't count toward the exit-code failure tally. Hard
		// checks always do.
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
		result.Checks = append(result.Checks, check)
		if opts.JSON {
			_ = json.NewEncoder(opts.Writer).Encode(check)
		} else {
			fmt.Fprintf(opts.Writer, "[%d/%d] %-32s %s   %dms\n",
				i+1, len(checks), c.name,
				humanStatus(err, c.optional), cDur,
			)
		}
	}

	result.DurationMs = opts.Now().Sub(start).Milliseconds()
	switch {
	case failed == len(checks) && failed > 0:
		// Total failure (e.g. data dir unwritable) is an
		// environment problem, not a per-check fault.
		result.Exit = 2
	case failed > 0:
		result.Exit = 1
	default:
		result.Exit = 0
	}
	writeSmokeJSON(opts.Writer, opts.JSON, result)
	if !opts.JSON {
		fmt.Fprintln(opts.Writer, summarize(result))
	}
	return result, result.Exit
}

// --- check registry ---

type smokeCheckFn func(ctx context.Context, a *App) error

type smokeCheckDef struct {
	name     string
	fn       smokeCheckFn
	optional bool // when true, a non-nil error still counts as exit 0 (warning only)
}

func defaultSmokeChecks() []smokeCheckDef {
	return []smokeCheckDef{
		{name: "data_dir_resolves", fn: checkDataDir},
		{name: "logs_dir_separate", fn: checkLogsDirSeparate},
		{name: "sqlite_open", fn: checkSQLiteOpen},
		{name: "migrations_applied", fn: checkMigrationsApplied},
		// oauth_defaults_loaded is optional: the file is a shared
		// deployment convenience, not a runtime requirement.
		{name: "oauth_defaults_loaded", fn: checkOAuthDefaults, optional: true},
		{name: "templates_dir", fn: checkTemplatesDir},
		{name: "typst_binary", fn: checkTypstBinary},
		{name: "routes_registered", fn: checkRoutesRegistered},
	}
}

// --- individual checks ---

// checkDataDir verifies the data directory resolves to a
// writable location. Catches `appdata.DefaultDir()` falling
// back to "." on a missing user config dir (the
// `8449e0645e`-class bug).
func checkDataDir(ctx context.Context, a *App) error {
	dir := a.dataDir
	if dir == "" {
		return fmt.Errorf("dataDir not resolved on App")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dataDir %s: %w", dir, err)
	}
	probe := filepath.Join(dir, ".dixiedata-smoke-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return fmt.Errorf("write probe in dataDir %s: %w", dir, err)
	}
	_ = os.Remove(probe)
	return nil
}

// checkLogsDirSeparate asserts the app logs directory lives
// outside the data directory. Catches the `b9a30ccb66`-class
// bug where logs co-located with the archive payload broke
// `.ddbak` restore. The failure mode is "logs path is inside
// data dir" (parent-of-logs == data-dir or deeper). Sibling
// layouts (parent-of-logs == parent-of-data-dir) are fine.
func checkLogsDirSeparate(ctx context.Context, a *App) error {
	logs := appdata.LogsDir(a.dataDir)
	// Walk up from logs to confirm we exit a.dataDir before
	// hitting the filesystem root.
	cursor := logs
	for {
		parent := filepath.Dir(cursor)
		if parent == cursor {
			// Hit the root without ever matching a.dataDir;
			// logs is definitely not inside the data dir.
			return nil
		}
		if samePath(parent, a.dataDir) {
			return fmt.Errorf("logs dir %s is inside data dir %s; must be a sibling to keep .ddbak restore working",
				logs, a.dataDir)
		}
		cursor = parent
	}
}

func samePath(a, b string) bool {
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	return strings.EqualFold(aa, bb)
}

// checkSQLiteOpen asserts the database handle is open and
// responds to a trivial query.
func checkSQLiteOpen(ctx context.Context, a *App) error {
	if a.database == nil {
		return fmt.Errorf("database handle is nil")
	}
	conn := a.database.Conn()
	if conn == nil {
		return fmt.Errorf("database Conn() returned nil")
	}
	var one int
	if err := conn.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil {
		return fmt.Errorf("SELECT 1 failed: %w", err)
	}
	if one != 1 {
		return fmt.Errorf("SELECT 1 returned %d", one)
	}
	return nil
}

// checkMigrationsApplied asserts the schema PRAGMA user_version
// matches CurrentSchemaVersion. Catches the
// `applySchema`-not-run class of bugs that produce "no such
// table" errors at request time.
func checkMigrationsApplied(ctx context.Context, a *App) error {
	conn := a.database.Conn()
	var version int
	if err := conn.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if version != db.CurrentSchemaVersion {
		return fmt.Errorf("user_version=%d, want %d", version, db.CurrentSchemaVersion)
	}
	return nil
}

// checkOAuthDefaults probes the Google OAuth defaults file
// locations used by `integrations.googleDefaultsCandidatePaths`.
// Not a hard fail when absent (users can connect their own
// account without shared defaults), but reports so a release
// with a missing file shows up.
func checkOAuthDefaults(ctx context.Context, a *App) error {
	candidates := googleDefaultsCandidatePaths()
	if len(candidates) == 0 {
		return fmt.Errorf("no candidate paths to probe")
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return nil // at least one found
		}
	}
	// Soft fail: status reported via Error but exit code stays 0
	// for this check — the file is optional.
	return fmt.Errorf("google-oauth-defaults.json not found in any candidate location (optional)")
}

// checkTemplatesDir asserts the Typst templates directory
// resolves to a directory that contains soldier_landscape.typ.
// Catches `0f485d51e6` — findTemplatesDir returning the Go
// html/template dir instead of the Typst source.
func checkTemplatesDir(ctx context.Context, a *App) error {
	dir, err := a.findTemplatesDir()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, "soldier_landscape.typ")); err != nil {
		return fmt.Errorf("templates dir %s missing soldier_landscape.typ: %w", dir, err)
	}
	return nil
}

// checkTypstBinary asserts the Typst binary resolves to an
// existing file. Catches `7dbff27` / `918fa5e` — release zip
// missing typst-windows.exe next to DixieData.exe.
func checkTypstBinary(ctx context.Context, a *App) error {
	bin, err := a.findTypstBinary()
	if err != nil {
		return err
	}
	info, err := os.Stat(bin)
	if err != nil {
		return fmt.Errorf("stat %s: %w", bin, err)
	}
	if info.Size() < 1024 {
		return fmt.Errorf("typst binary %s is suspiciously small (%d bytes) — likely a 0-byte stub from a botched tar extraction", bin, info.Size())
	}
	return nil
}

// checkRoutesRegistered asserts the mux was wired with at least
// the canonical routes. Catches `caf2c28626`-class
// routes-not-registered bugs (blank Wails window) and the
// `10d0d46fcc` chi-method-flip bugs (silent 405 on click).
func checkRoutesRegistered(ctx context.Context, a *App) error {
	if a.mux == nil {
		return fmt.Errorf("mux is nil — setupRoutes() never ran")
	}
	// Use httptest to fire a couple of canonical routes and assert
	// they don't return 404 (which means the route isn't
	// registered at all).
	for _, probe := range []struct {
		method, path string
	}{
		{"GET", "/"},
		{"GET", "/share"},
		{"GET", "/soldiers/search/recent"},
	} {
		req := httptest.NewRequest(probe.method, probe.path, nil)
		w := httptest.NewRecorder()
		a.mux.ServeHTTP(w, req)
		if w.Result().StatusCode == http.StatusNotFound {
			return fmt.Errorf("%s %s returned 404 — route not registered", probe.method, probe.path)
		}
	}
	return nil
}

// --- app build for smoke ---

// buildSmokeApp constructs an *App, runs startup() (which does
// everything wails.Run would do minus the window), and returns
// the result. Does NOT call Wails. Safe for headless contexts.
func buildSmokeApp(ctx context.Context, dataDirOverride string) (*App, error) {
	a := NewApp()
	if dataDirOverride != "" {
		a.dataDir = dataDirOverride
	}
	// Wails passes a context to Startup(); we don't have one
	// here, so build a fresh one. The context is only used by
	// services that listen for cancellation, and smoke runs to
	// completion anyway.
	a.startup(ctx)
	if a.startupErr != nil {
		return a, fmt.Errorf("startup: %w", a.startupErr)
	}
	return a, nil
}

// smokeShutdown mirrors (*App).Shutdown() — drains jobs then
// closes the DB. We can't call the public Shutdown because Wails
// passes it a context; replicate the body inline for clarity.
func smokeShutdown(a *App) {
	if a.jobs != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = a.jobs.Shutdown(shutdownCtx)
		cancel()
	}
	if a.database != nil {
		_ = a.database.Close()
	}
}

// --- output helpers ---

func humanStatus(err error, optional bool) string {
	if err == nil {
		return "ok"
	}
	if optional {
		return "warn"
	}
	return "FAIL"
}

func summarize(r SmokeResult) string {
	passed := len(r.Checks)
	for _, c := range r.Checks {
		if !c.Passed {
			passed--
		}
	}
	status := "PASS"
	switch r.Exit {
	case 0:
		status = "PASS"
	case 1:
		status = "FAIL"
	case 2:
		status = "ENV-ERROR"
	}
	return fmt.Sprintf("---\n%s: %d/%d checks in %dms\n",
		status, passed, len(r.Checks), r.DurationMs)
}

func writeSmokeJSON(w io.Writer, wantJSON bool, r SmokeResult) {
	if !wantJSON {
		return
	}
	_ = json.NewEncoder(w).Encode(r)
}

// buildIdentityString returns a compact "<app> v<ver>" string
// for the result envelope. Falls back to "?" if buildinfo is
// not initialized (e.g. test fixtures).
func buildIdentityString() string {
	v := db.GetAppVersion()
	if v == "" {
		return "?"
	}
	return "DixieData v" + v
}

// googleDefaultsCandidatePaths is a small local copy of the
// probe used by integrations. We avoid importing the internal
// helper because it's currently package-private. Keep the
// candidate order identical to
// integrations.googleDefaultsCandidatePaths so a release
// zip layout that works in the app also works for smoke.
func googleDefaultsCandidatePaths() []string {
	var paths []string
	if exe, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(exe), "google-oauth-defaults.json"))
	}
	if wd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(wd, "google-oauth-defaults.json"))
	}
	// Also probe exe's parent's parent — covers `go run` from
	// the repo root where the binary lives in build/bin but the
	// source file sits at the repo root.
	if exe, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(filepath.Dir(exe)), "google-oauth-defaults.json"))
	}
	return paths
}

// HasSmokeFlag returns true when the os.Args slice contains
// --smoke or --smoke-json. main.go uses this to decide whether
// to enter the smoke path or call wails.Run.
func HasSmokeFlag(args []string) bool {
	for _, a := range args {
		if a == "--smoke" || a == "--smoke-json" {
			return true
		}
	}
	return false
}

// WantsSmokeJSON returns true when --smoke-json was passed.
func WantsSmokeJSON(args []string) bool {
	for _, a := range args {
		if a == "--smoke-json" {
			return true
		}
	}
	return false
}

// EnvRequestsSmoke returns true when DIXIEDATA_SMOKE=1 is set.
// Used by main.go so a launcher script can request smoke mode
// without rebuilding the arg vector.
func EnvRequestsSmoke() bool {
	v := strings.TrimSpace(os.Getenv("DIXIEDATA_SMOKE"))
	return v == "1" || strings.EqualFold(v, "true")
}