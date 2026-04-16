package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed build/appicon.png
var icon []byte

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create an instance of the app structure
	app := NewApp()

	// Create application with options https://wails.io/docs/reference/options
	err := wails.Run(&options.App{
		Title:         "AppMonitor",
		Width:         1024,
		Height:        900,
		DisableResize: false,
		Fullscreen:    false,
		// WindowStartState:  options.Maximised,
		Frameless:         false,
		MinWidth:          850,
		MinHeight:         650,
		MaxWidth:          1920,
		MaxHeight:         1080,
		StartHidden:       false,
		HideWindowOnClose: false,
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarDefault(),
			Appearance:           mac.DefaultAppearance,
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			About: &mac.AboutInfo{
				Title:   "AppMonitor",
				Message: "© 2026 AppMonitor",
				Icon:    icon,
			},
		},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
