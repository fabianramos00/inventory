//go:build bindings

package main

import (
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
)

func main() {
	app := NewApp("http://127.0.0.1:0", nil)
	if err := wails.Run(newWailsOptions(app)); err != nil {
		fmt.Fprintln(os.Stderr, "wails bindings generation failed:", err)
		os.Exit(1)
	}
}
