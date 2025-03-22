package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func NewAddressBookScreen() fyne.CanvasObject {
	list := widget.NewList(
		func() int { return 5 },
		func() fyne.CanvasObject { return widget.NewLabel("Template") },
		func(id widget.ListItemID, item fyne.CanvasObject) {
			item.(*widget.Label).SetText("Address " + string(id))
		},
	)
	return container.NewVBox(
		widget.NewLabel("Address Book"),
		list,
		widget.NewButton("Add New Address", func() {}),
	)
}