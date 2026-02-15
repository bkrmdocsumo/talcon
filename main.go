package main

import (
	"embed"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func setupLogFile() (*os.File, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home dir: %w", err)
	}
	logDir := filepath.Join(home, ".talon")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", logDir, err)
	}
	logPath := filepath.Join(logDir, "talon.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	return f, nil
}

func main() {
	logFile, err := setupLogFile()
	if err != nil {
		log.Printf("Warning: could not set up log file: %v", err)
	} else {
		defer logFile.Close()
	}

	app := NewApp()

	err = wails.Run(&options.App{
		Title:             "Talon",
		Width:             1024,
		Height:            768,
		MinWidth:          800,
		MinHeight:         600,
		HideWindowOnClose: true,
		BackgroundColour:  &options.RGBA{R: 10, G: 10, B: 15, A: 255},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: true,
				HideTitle:                 true,
				HideTitleBar:              false,
				FullSizeContent:           true,
			},
			WebviewIsTransparent: true,
			WindowIsTranslucent:  false,
			Appearance:           mac.NSAppearanceNameDarkAqua,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
