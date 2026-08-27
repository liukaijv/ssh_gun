package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"ssh_gun/internal/rshbridge"
)

//go:embed all:frontend/dist
var assets embed.FS

// cwRsync is kept separate from frontend assets so it can be extracted into a
// normal Windows directory where rsync can locate its companion DLLs.
//
//go:embed all:assets/cwrsync_6.4.8_x64
var cwrsyncAssets embed.FS

func main() {
	if rshbridge.IsBridgeMode(os.Args) {
		if err := rshbridge.Run(os.Args); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "ssh_gun rsh bridge:", err)
			os.Exit(1)
		}
		return
	}

	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "飞梭", // overridden at startup from saved language via WindowSetTitle
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnBeforeClose:    app.beforeClose,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
