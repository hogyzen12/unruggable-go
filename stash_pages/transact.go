package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func NewTransactScreen() fyne.CanvasObject {
	return container.NewVBox(
		widget.NewLabel("Transact Screen"),
		widget.NewLabel("Implement transaction functionality here"),
	)
}