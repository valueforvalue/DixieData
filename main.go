package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/appshell"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed frontend
var assets embed.FS

func main() {
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
		_, code := appshell.RunSmoke(context.Background(), appshell.SmokeOptions{
			JSON: appshell.WantsSmokeJSON(os.Args[1:]),
		})
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

	err = wails.Run(&options.App{
		Title:  fmt.Sprintf("DixieData v%s", db.GetAppVersion()),
		Width:  1280,
		Height: 800,
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

// runQuerySubcommand builds an App, parses the query args,
// dispatches, and returns the exit code. The App is fully
// started (so the soldiers facade is wired) then shut down so
// background jobs + the DB close cleanly. We don't need Wails.
func runQuerySubcommand() int {
	opts, _ := appshell.ParseQueryCommand(os.Args[1:])
	a := appshell.NewApp()
	ctx := context.Background()
	a.Startup(ctx)
	defer a.Shutdown(ctx)
	opts.App = a
	code, err := appshell.RunQuery(ctx, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	return code
}

// runExportSubcommand builds an App, parses export args, dispatches
// to RunExport, returns the exit code. Same lifecycle as
// runQuerySubcommand. No Wails — bypasses the native SaveFileDialog
// entirely (every command takes --out PATH).
func runExportSubcommand() int {
	if dir := firstDataDir(os.Args[1:]); dir != "" {
		_ = os.Setenv("DIXIEDATA_DATA_DIR", dir)
	}
	opts, err := appshell.ParseExportArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 3
	}
	a := appshell.NewApp()
	ctx := context.Background()
	a.Startup(ctx)
	defer a.Shutdown(ctx)
	opts.App = a
	code, err := appshell.RunExport(ctx, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	return code
}

// runImportSubcommand mirrors runExportSubcommand. Same lifecycle.
// No Wails — bypasses the native OpenFileDialog entirely (every
// command takes --from PATH).
func runImportSubcommand() int {
	if dir := firstDataDir(os.Args[1:]); dir != "" {
		_ = os.Setenv("DIXIEDATA_DATA_DIR", dir)
	}
	opts, err := appshell.ParseImportArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 3
	}
	a := appshell.NewApp()
	ctx := context.Background()
	a.Startup(ctx)
	defer a.Shutdown(ctx)
	opts.App = a
	code, err := appshell.RunImport(ctx, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	return code
}

// runAdminSubcommand handles the Phase 6 admin subcommand
// families: migrate / backup / restore point / logs / config.
// Same lifecycle as runExport/Import. --data-dir is honoured
// by setting DIXIEDATA_DATA_DIR before a.Startup() so
// appdata.DefaultDir() picks it up.
func runAdminSubcommand() int {
	args := os.Args[1:]
	if dir := firstDataDir(args); dir != "" {
		_ = os.Setenv("DIXIEDATA_DATA_DIR", dir)
	}
	opts, err := appshell.ParseAdminArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 3
	}
	a := appshell.NewApp()
	ctx := context.Background()
	a.Startup(ctx)
	defer a.Shutdown(ctx)
	opts.App = a
	code, err := appshell.RunAdmin(ctx, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
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
func runDebugSubcommand() int {
	opts, err := appshell.ParseDebugArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 3
	}
	if err := appshell.ApplyDebugDataDirOverride(opts.DataDir); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 3
	}
	a := appshell.NewApp()
	ctx := context.Background()
	a.Startup(ctx)
	defer a.Shutdown(ctx)
	opts.App = a
	code, err := appshell.RunDebug(ctx, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	return code
}
