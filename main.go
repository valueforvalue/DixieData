package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"

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
	// (doctor), Phase 3 (list / show / search) of
	// docs/agents/cli-plan.md. Smoke is a flag-style invocation;
	// doctor + query subcommands are positional verbs.
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
	if appshell.HasSmokeFlag(os.Args[1:]) || appshell.EnvRequestsSmoke() {
		_, code := appshell.RunSmoke(context.Background(), appshell.SmokeOptions{
			JSON: appshell.WantsSmokeJSON(os.Args[1:]),
		})
		os.Exit(code)
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
	})
	if err != nil {
		panic(err)
	}
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
