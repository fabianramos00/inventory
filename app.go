package main

import (
	"context"
	"embed"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

type App struct {
	mu           sync.RWMutex
	apiBaseURL   string
	shutdownOnce sync.Once
	shutdownFn   func(context.Context)
}

func NewApp(apiBaseURL string, shutdown func(context.Context)) *App {
	return &App{apiBaseURL: apiBaseURL, shutdownFn: shutdown}
}

func (a *App) GetAPIBaseURL() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.apiBaseURL
}

func (a *App) shutdown(ctx context.Context) {
	a.shutdownOnce.Do(func() {
		if a.shutdownFn != nil {
			a.shutdownFn(ctx)
		}
	})
}

func newWailsOptions(app *App) *options.App {
	return &options.App{
		Title:     "Inventory Desktop",
		Width:     1280,
		Height:    800,
		MinWidth:  1024,
		MinHeight: 700,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnShutdown: app.shutdown,
		Bind:       []interface{}{app},
	}
}
