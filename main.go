package main

import (
	"embed"
	"os"

	"github.com/valueforvalue/DixieData/internal/uiids"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed frontend
var assets embed.FS

func main() {
	uiids.EnableFromArgs(os.Args[1:])

	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "DixieData",
		Width:  1280,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: app,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
	})
	if err != nil {
		panic(err)
	}
}
