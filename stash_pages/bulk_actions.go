package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func NewBulkActionsScreen() fyne.CanvasObject {
	return container.NewVBox(
		widget.NewLabel("Bulk Actions"),
		widget.NewButton("Multi Swap", func() {}),
		widget.NewButton("Multi Send", func() {}),
	)
}