package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/appshell"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/config"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed frontend
var assets embed.FS

// handleVersionFlag scans argv for --version / -v. Returns
// the formatted output + done=true if found, done=false
// otherwise. Exposed as a separate function so the test
// (main_test.go) can verify the behaviour without calling
// main() (which calls os.Exit).
func handleVersionFlag(argv []string) (output string, done bool) {
	for _, a := range argv[1:] {
		if a == "--version" || a == "-v" {
			// Issue #370: include the codename + branch so a
			// user (or a support ticket) can identify the
			// exact build. Shape:
			//   DixieData v1.1.65 · First Manassas
			//   dev · commit abc1234 · 2026-07-08T...Z
			branch := buildinfo.GitBranch
			if strings.TrimSpace(branch) == "" {
				branch = "unknown"
			}
			return fmt.Sprintf("%s · %s\n%s\n",
				buildinfo.AppLabel(),
				buildinfo.ReleaseLabel(),
				branch+" · "+buildinfo.BuildIdentity(),
			), true
		}
	}
	return "", false
}

// windowTitle is the OS window title for the Wails desktop
// app. Composed at the call site (issue #542) so the brand +
// codename boundary uses an em dash, matching the typographic
// polish issue #462 applied to the per-page <title> + the
// top-shell brand pill + the footer. The "DixieData —
// {codename} · {version} · {branch}" shape reads cleanly at
// the top of the OS window frame on every platform (Windows /
// macOS / Linux).
//
// We use buildinfo.Codename() rather than buildinfo.ReleaseLabel()
// here because the latter returns "DixieData {codename}" with
// a literal space — the em-dash polish would require editing
// the source-of-truth function, which would ripple to every
// ReleaseLabel() consumer (--version output, footer) and force
// a coordinated edit of the footer's call site (which composes
// its own em-dash separator per #462). Composing at the call
// site keeps the change contained to the OS window title, the
// surface the user reported.
//
// The middle dots (`·`) between codename + version + branch
// stay as-is — those bind the metadata chain, not the brand /
// codename boundary (per #462).
func windowTitle() string {
	return fmt.Sprintf("DixieData — %s · %s · %s",
		buildinfo.Codename(),
		buildinfo.AppVersion,
		buildinfo.GitBranch,
	)
}

// handleHelpFlag scans argv for the explicit help token
// (help / --help / -h). Returns the formatted help text +
// requested=true. The no-args case is intentionally NOT
// handled here: a bare `dixiedata` invocation must fall
// through to the GUI launch path. If we showed help on
// no-args, the Wails GUI would never start.
//
// The list of subcommands is hand-maintained; main_test.go
// asserts that every verb the dispatcher knows about appears
// in the help text (cross-reference against
// runDebugCLICoverage). See issue #277.
func handleHelpFlag(argv []string) (output string, requested bool) {
	for _, a := range argv[1:] {
		if a == "help" || a == "--help" || a == "-h" {
			return cliHelpText(), true
		}
	}
	return "", false
}

// cliHelpText is the hand-maintained help output. main_test.go
// asserts that every verb listed here is also recognised by
// the dispatcher (catches drift between the help text and
// the actual surface).
func cliHelpText() string {
	return `DixieData CLI — headless archive operations

Usage:
  dixiedata <subcommand> [flags]
  dixiedata --version | --help

Subcommands:
  --smoke             Headless boot check (8 checks)
  --version           Print app version + build identity
  doctor              Diagnose the local install
  list                List records (soldiers, sources)
  show                Show a single record
  search              Search across records
  soldier             Mutate a soldier (create; update/delete follow)
  export              Export PDFs / JPGs / JSON / CSV / iCal / archives
  import              Import .ddbak / .ddshare / images / memorial-json
  migrate             Apply / inspect schema migrations
  backup              List / prune retained backups
  restore point       Manage restore points
  logs                Tail / locate app logs
  config              Show / set local settings
  debug               Dump / hx-invariants / browser-tree / request
                      cli-coverage / in-place-safety

For per-subcommand flags, run ` + "`dixiedata <subcommand> --help`." + `

See docs/agents/cli-plan.md for the full roadmap.
`
}

func main() {
	// --version / -v short-circuit. Sits BEFORE the
	// subcommand dispatchers so it doesn't open the DB or
	// start the appshell. The user gets the version + build
	// identity and a clean exit. See issue #271.
	if output, done := handleVersionFlag(os.Args); done {
		fmt.Print(output)
		os.Exit(0)
	}

	// help / --help / -h. Lists every subcommand with a
	// one-line description. Sits BEFORE the subcommand
	// dispatchers so it doesn't open the DB either. See
	// issue #277.
	if output, requested := handleHelpFlag(os.Args); requested {
		fmt.Print(output)
		os.Exit(0)
	}

	// --log-to-stderr: tee the JSONL to stderr for shell-side
	// CI debugging. Issue #270. Set the env var BEFORE the
	// appshell starts (lifecycle.go reads it after
	// debug.Configure).
	if hasLogToStderr(os.Args[1:]) {
		_ = os.Setenv("DIXIEDATA_LOG_TO_STDERR", "1")
	}

	// Headless subcommand dispatch. Phase 1 (--smoke), Phase 2
	// (doctor), Phase 3 (list / show / search), Phase 4 (export),
	// Phase 5 (import), Phase 6 (migrate/backup/restore point/
	// logs/config) of docs/agents/cli-plan.md. Smoke is a
	// flag-style invocation; the rest are positional verbs.
	if appshell.HasDoctorFlag(os.Args[1:]) {
		_, code := appshell.RunDoctor(context.Background(), appshell.DoctorOptions{
			JSON:   appshell.WantsDoctorJSON(os.Args[1:]),
			Fix:    appshell.WantsDoctorFix(os.Args[1:]),
			Checks: appshell.ParseDoctorChecks(os.Args[1:]),
		})
		os.Exit(code)
	}
	if appshell.HasQuerySubcommand(os.Args[1:]) {
		code := runQuerySubcommand()
		os.Exit(code)
	}
	if appshell.HasMutateSubcommand(os.Args[1:]) {
		code := runMutateSubcommand()
		os.Exit(code)
	}
	if appshell.HasExportSubcommand(os.Args[1:]) {
		code := runExportSubcommand()
		os.Exit(code)
	}
	if appshell.HasImportSubcommand(os.Args[1:]) {
		code := runImportSubcommand()
		os.Exit(code)
	}
	if appshell.HasAdminSubcommand(os.Args[1:]) {
		code := runAdminSubcommand()
		os.Exit(code)
	}
	if appshell.HasDebugSubcommand(os.Args[1:]) {
		code := runDebugSubcommand()
		os.Exit(code)
	}
	if appshell.HasSmokeFlag(os.Args[1:]) || appshell.EnvRequestsSmoke() {
		// Issue #597: signal-aware ctx so SIGINT/SIGTERM cancel
		// the smoke run cleanly and the deferred smokeShutdown
		// inside RunSmoke actually fires (closes the DB).
		// Mirrors the lifecycle pattern in the subcommand
		// runners above.
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		_, code := appshell.RunSmoke(ctx, appshell.SmokeOptions{
			JSON: appshell.WantsSmokeJSON(os.Args[1:]),
		})
		os.Exit(code)
	}
	if appshell.HasSeedFlag(os.Args[1:]) || appshell.EnvRequestsSeed() {
		// Issue #667 follow-up (flag form before the future
		// `dixiedata seed` positional subcommand lands). Same
		// signal-aware ctx pattern as --smoke so Ctrl+C cancels
		// cleanly mid-seed.
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		opts := appshell.ParseSeedOptions(os.Args[1:])
		opts.JSON = appshell.WantsSeedJSON(os.Args[1:])
		_, code := appshell.RunSeed(ctx, opts)
		os.Exit(code)
	}

	// waitForDebugger pauses the process until a debugger
	// attaches or the user Ctrl-Cs out. Used by Run-DixieData-
	// Debug.ps1: set DIXIEDATA_WAIT_FOR_DEBUGGER=1 in your shell
	// before launch, then `dlv attach $PID` from another shell
	// before the 30s timeout fires. Useful when reproducing a
	// crash in Startup() — you want breakpoints on first line.
	if os.Getenv("DIXIEDATA_WAIT_FOR_DEBUGGER") == "1" {
		fmt.Fprintln(os.Stderr, "DIXIEDATA_WAIT_FOR_DEBUGGER=1: pausing for 30s; attach with `dlv attach $PID` from another shell.")
		fmt.Fprintf(os.Stderr, "pid=%d\n", os.Getpid())
		time.Sleep(30 * time.Second)
	}

	frontendAssets, err := fs.Sub(assets, "frontend")
	if err != nil {
		panic(err)
	}

	app := appshell.NewApp().WithFrontendAssets(frontendAssets)

	// Load application config early so window size is honored
	// before Wails.Run opens the OS window (issues #636-#639).
	// Data dir resolution matches appdata.DefaultDir().
	appCfg := config.Defaults()
	if loaded, err := config.Load(appdata.DefaultDir()); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load app config, using defaults: %v\n", err)
	} else {
		appCfg = loaded
	}

	err = wails.Run(&options.App{
		Title:  windowTitle(),
		Width:  appCfg.Window.Width,
		Height: appCfg.Window.Height,
		Bind: []interface{}{
			app,
		},
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: app,
		},
		OnStartup:  app.Startup,
		OnShutdown: app.Shutdown,
		// EnableDefaultContextMenu turns on the browser's default
		// context menu in production builds so the user can right-click
		// → "Inspect" to open DevTools. In debug builds (`wails build
		// -debug`) this is already on; setting it here is a no-op.
		// The DIXIEDATA_DEVTOOLS=1 env var (set by the debug launcher)
		// forces this on even in a release build so a user can debug
		// without rebuilding.
		EnableDefaultContextMenu: os.Getenv("DIXIEDATA_DEVTOOLS") == "1",
	})
	if err != nil {
		panic(err)
	}
}

// firstDataDir scans args for --data-dir PATH / --data-dir=PATH
// and returns the path, or "" if absent. Centralised so all
// subcommand helpers honour the same flag without re-implementing
// the scan. The path is returned verbatim — no canonicalisation —
// because appdata.DefaultDir() does the clean/join downstream.
func firstDataDir(args []string) string {
	for i, a := range args {
		if a == "--data-dir" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, "--data-dir=") {
			return strings.TrimPrefix(a, "--data-dir=")
		}
	}
	return ""
}

// hasLogToStderr scans args for --log-to-stderr. Returns true
// if present (any form: --log-to-stderr, --log-to-stderr=1).
// Issue #270.
func hasLogToStderr(args []string) bool {
	for _, a := range args {
		if a == "--log-to-stderr" {
			return true
		}
		if a == "--log-to-stderr=0" || a == "--log-to-stderr=false" {
			return false
		}
		if strings.HasPrefix(a, "--log-to-stderr=") {
			return true
		}
	}
	return false
}

// runQuerySubcommand builds an App, parses the query args,
// dispatches, and returns the exit code. The App is fully
// started (so the soldiers facade is wired) then shut down so
// background jobs + the DB close cleanly. We don't need Wails.
//
// Issue #597: the lifecycle ctx is signal-aware on POSIX. SIGINT
// / SIGTERM cancel the ctx so the deferred Shutdown runs and the
// SQLite DB closes cleanly — no straggling dixiedata.db-wal /
// dixiedata.db-shm sidecar files. signal.NotifyContext is the
// Go 1.16+ canonical helper; stop() releases the handler on
// normal exit so it does not leak across subsequent invocations.
func runQuerySubcommand() int {
	code, err := recoverExit5(func() (int, error) {
		opts, _ := appshell.ParseQueryCommand(os.Args[1:])
		a := appshell.NewApp()
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		a.Startup(ctx)
		defer a.Shutdown(ctx)
		opts.App = a
		return appshell.RunQuery(ctx, opts)
	})
	if err != nil && code != 5 {
		writeError(os.Stderr, err.Error())
	}
	return code
}

// runMutateSubcommand builds an App, parses the mutate args,
// dispatches to RunMutate, and returns the exit code. Same
// lifecycle as runQuerySubcommand. Phase 8 of cli-plan.md
// (issue #371) — write verbs split from read-only queries
// because the mutate surface will grow (update / delete /
// event create / tag-attach / etc.) and doesn't belong in
// the cli_query.go file.
//
// Issue #597: lifecycle ctx is signal-aware on POSIX (see
// runQuerySubcommand for the rationale).
func runMutateSubcommand() int {
	code, err := recoverExit5(func() (int, error) {
		if dir := firstDataDir(os.Args[1:]); dir != "" {
			_ = os.Setenv("DIXIEDATA_DATA_DIR", dir)
		}
		opts, ok := appshell.ParseMutateCommand(os.Args[1:])
		if !ok {
			return 3, fmt.Errorf("usage: dixiedata soldier create --from <path> | --from-stdin")
		}
		a := appshell.NewApp()
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		a.Startup(ctx)
		defer a.Shutdown(ctx)
		opts.App = a
		return appshell.RunMutate(ctx, opts)
	})
	if err != nil && code != 5 {
		writeError(os.Stderr, err.Error())
	}
	return code
}

// runExportSubcommand builds an App, parses export args, dispatches
// to RunExport, returns the exit code. Same lifecycle as
// runQuerySubcommand. No Wails — bypasses the native SaveFileDialog
// entirely (every command takes --out PATH).
//
// Issue #597: lifecycle ctx is signal-aware on POSIX (see
// runQuerySubcommand for the rationale).
func runExportSubcommand() int {
	code, err := recoverExit5(func() (int, error) {
		if dir := firstDataDir(os.Args[1:]); dir != "" {
			_ = os.Setenv("DIXIEDATA_DATA_DIR", dir)
		}
		opts, err := appshell.ParseExportArgs(os.Args[1:])
		if err != nil {
			return 3, err
		}
		a := appshell.NewApp()
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		a.Startup(ctx)
		defer a.Shutdown(ctx)
		opts.App = a
		return appshell.RunExport(ctx, opts)
	})
	if err != nil && code != 5 {
		writeError(os.Stderr, err.Error())
	}
	return code
}

// runImportSubcommand mirrors runExportSubcommand. Same lifecycle.
// No Wails — bypasses the native OpenFileDialog entirely (every
// command takes --from PATH).
//
// Issue #597: lifecycle ctx is signal-aware on POSIX (see
// runQuerySubcommand for the rationale).
func runImportSubcommand() int {
	if dir := firstDataDir(os.Args[1:]); dir != "" {
		_ = os.Setenv("DIXIEDATA_DATA_DIR", dir)
	}
	code, err := recoverExit5(func() (int, error) {
		opts, err := appshell.ParseImportArgs(os.Args[1:])
		if err != nil {
			return 3, err
		}
		a := appshell.NewApp()
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		a.Startup(ctx)
		defer a.Shutdown(ctx)
		opts.App = a
		return appshell.RunImport(ctx, opts)
	})
	if err != nil && code != 5 {
		writeError(os.Stderr, err.Error())
	}
	return code
}

// runAdminSubcommand handles the Phase 6 admin subcommand
// families: migrate / backup / restore point / logs / config.
// Same lifecycle as runExport/Import. --data-dir is honoured
// by setting DIXIEDATA_DATA_DIR before a.Startup() so
// appdata.DefaultDir() picks it up.
//
// Issue #597: lifecycle ctx is signal-aware on POSIX (see
// runQuerySubcommand for the rationale).
func runAdminSubcommand() int {
	code, err := recoverExit5(func() (int, error) {
		args := os.Args[1:]
		if dir := firstDataDir(args); dir != "" {
			_ = os.Setenv("DIXIEDATA_DATA_DIR", dir)
		}
		opts, err := appshell.ParseAdminArgs(args)
		if err != nil {
			return 3, err
		}
		a := appshell.NewApp()
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		a.Startup(ctx)
		defer a.Shutdown(ctx)
		opts.App = a
		return appshell.RunAdmin(ctx, opts)
	})
	if err != nil && code != 5 {
		writeError(os.Stderr, err.Error())
	}
	return code
}

// runDebugSubcommand wires Phase 7 of cli-plan.md (debug ...).
// Same lifecycle as runImportSubcommand — build *App, call
// Startup, dispatch, call Shutdown. Honours --data-dir by
// setting DIXIEDATA_DATA_DIR before constructing the App so
// appdata.DefaultDir() inside startup() picks it up.
//
// Debug subcommands are strictly read-only. They never accept
// --yes and never touch the archive file. Useful for support
// workflows where the GUI is unavailable.
//
// Issue #597: lifecycle ctx is signal-aware on POSIX (see
// runQuerySubcommand for the rationale).
func runDebugSubcommand() int {
	code, err := recoverExit5(func() (int, error) {
		opts, err := appshell.ParseDebugArgs(os.Args[1:])
		if err != nil {
			return 3, err
		}
		if err := appshell.ApplyDebugDataDirOverride(opts.DataDir); err != nil {
			return 3, err
		}
		a := appshell.NewApp()
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		a.Startup(ctx)
		defer a.Shutdown(ctx)
		opts.App = a
		return appshell.RunDebug(ctx, opts)
	})
	if err != nil && code != 5 {
		writeError(os.Stderr, err.Error())
	}
	return code
}
