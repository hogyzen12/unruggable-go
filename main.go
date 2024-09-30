package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"unruggable-go/internal/ui"
)

func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("Unruggable")

	// Initialize global state
	state := ui.GetGlobalState()

	sidebar := ui.NewSidebar()
	content := container.NewMax()

	walletTabs := ui.NewWalletTabs(func(wallet string) {
		state.SetSelectedWallet(wallet)
		sidebar.OnHomeClicked() // Refresh home screen
	})

	// Load initial wallets
	walletManager := ui.NewWalletManager(myWindow, walletTabs)
	walletTabs.Update(walletManager.GetWallets())

	mainContent := container.NewBorder(walletTabs.Container(), nil, nil, nil, content)
	split := container.NewHSplit(sidebar, mainContent)
	split.Offset = 0.2

	myWindow.SetContent(split)
	myWindow.Resize(fyne.NewSize(800, 600)) // Increased size for better visibility

	sidebar.OnHomeClicked = func() {
		content.Objects = []fyne.CanvasObject{ui.NewHomeScreen()}
		content.Refresh()
	}
	sidebar.OnWalletClicked = func() {
		content.Objects = []fyne.CanvasObject{walletManager.NewWalletScreen()}
		content.Refresh()
	}
	sidebar.OnAddressBookClicked = func() {
		content.Objects = []fyne.CanvasObject{ui.NewAddressBookScreen()}
		content.Refresh()
	}
	sidebar.OnSwapClicked = func() {
		content.Objects = []fyne.CanvasObject{ui.NewSwapScreen()}
		content.Refresh()
	}
	sidebar.OnBulkActionsClicked = func() {
		content.Objects = []fyne.CanvasObject{ui.NewBulkActionsScreen()}
		content.Refresh()
	}
	sidebar.OnCalypsoClicked = func() {
		content.Objects = []fyne.CanvasObject{ui.NewCalypsoScreen(myWindow)}
		content.Refresh()
	}
	sidebar.OnTxHistoryClicked = func() {
		content.Objects = []fyne.CanvasObject{ui.NewTxHistoryScreen()}
		content.Refresh()
	}
	sidebar.OnUnruggableClicked = func() {
		content.Objects = []fyne.CanvasObject{ui.NewUnruggableScreen()}
		content.Refresh()
	}

	sidebar.OnHomeClicked() // Start with the home screen

	myWindow.ShowAndRun()
}