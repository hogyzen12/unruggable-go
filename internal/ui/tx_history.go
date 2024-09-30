package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func NewTxHistoryScreen() fyne.CanvasObject {
	list := widget.NewList(
		func() int { return 10 },
		func() fyne.CanvasObject { return widget.NewLabel("Template") },
		func(id widget.ListItemID, item fyne.CanvasObject) {
			item.(*widget.Label).SetText("Transaction " + string(id))
		},
	)
	return container.NewVBox(
		widget.NewLabel("Transaction History"),
		list,
	)
}