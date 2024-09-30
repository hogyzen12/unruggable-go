package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func NewUnruggableScreen() fyne.CanvasObject {
	return container.NewVBox(
		widget.NewLabel("Unruggable MPC Setup"),
		widget.NewButton("Generate MPC Key", func() {}),
		widget.NewButton("Sign Transaction", func() {}),
	)
}