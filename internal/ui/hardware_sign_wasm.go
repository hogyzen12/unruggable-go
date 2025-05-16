//go:build js || wasm
// +build js wasm

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// NewSignScreen is not available in the Web build.
func NewSignScreen() fyne.CanvasObject {
	return widget.NewLabel("Hardware signing is not supported in the web version.")
}
