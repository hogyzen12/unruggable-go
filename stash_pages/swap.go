package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func NewSwapScreen() fyne.CanvasObject {
	fromTokenEntry := widget.NewEntry()
	fromTokenEntry.SetPlaceHolder("From Token")

	toTokenEntry := widget.NewEntry()
	toTokenEntry.SetPlaceHolder("To Token")

	amountEntry := widget.NewEntry()
	amountEntry.SetPlaceHolder("Amount")

	return container.NewVBox(
		widget.NewLabel("Swap Tokens"),
		fromTokenEntry,
		toTokenEntry,
		amountEntry,
		widget.NewButton("Swap", func() {}),
	)
}