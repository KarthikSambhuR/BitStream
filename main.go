//go:build !native

package main

import (
	"context"
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	cfg, hasWindowConfig := loadWindowConfig()
	width := 1220
	height := 760
	startState := options.Maximised
	if hasWindowConfig {
		width = cfg.Width
		height = cfg.Height
		startState = options.Normal
		if cfg.Maximized {
			startState = options.Maximised
		}
	}

	err := wails.Run(&options.App{
		Title:            "BitStream",
		Width:            width,
		Height:           height,
		MinWidth:         980,
		MinHeight:        620,
		Frameless:        true,
		CSSDragProperty:  "--wails-draggable",
		CSSDragValue:     "drag",
		BackgroundColour: options.NewRGB(10, 13, 18),
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnDomReady: func(ctx context.Context) {
			if hasWindowConfig && !cfg.Maximized {
				wailsruntime.WindowSetPosition(ctx, cfg.X, cfg.Y)
			}
		},
		DisableResize:     false,
		HideWindowOnClose: false,
		WindowStartState:  startState,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			Theme:                windows.Dark,
			BackdropType:         windows.None,
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			WebviewGpuIsDisabled: false,
			ResizeDebounceMS:     80,
			IsZoomControlEnabled: false,
			DisablePinchZoom:     true,
			EnableSwipeGestures:  false,
			DisableWindowIcon:    false,
			WindowClassName:      "BitStreamWailsWindow",
			DLLSearchPaths:       windows.DLLSearchSystem32 | windows.DLLSearchApplicationDir,
			ContentProtection:    false,
			WebviewUserDataPath:  "",
			WebviewBrowserPath:   "",
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
