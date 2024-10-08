package main

import (
	"fmt"
	"unruggable-go/internal/ui"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
)

func main() {
	myApp := app.NewWithID("com.unruggable.app")
	myWindow := myApp.NewWindow("Unruggable")

	var walletManager *ui.WalletManager

	walletTabs := ui.NewWalletTabs(func(walletID string) {
		fmt.Println("Switched to wallet:", walletID)
		if walletManager != nil {
			walletManager.SetSelectedWallet(walletID)
		}
	})

	walletManager = ui.NewWalletManager(myWindow, walletTabs, myApp)

	// Create a container for the main content
	mainContent := container.NewStack()

	// Add the WalletTabs to the bottom of the screen
	content := container.NewBorder(nil, walletTabs.Container(), nil, nil, mainContent)

	sidebar := ui.NewSidebar()

	split := container.NewHSplit(sidebar, content)
	split.SetOffset(0.2)

	myWindow.SetContent(split)

	updateMainContent := func(newContent fyne.CanvasObject) {
		mainContent.RemoveAll()
		mainContent.Add(newContent)
	}

	sidebar.OnHomeClicked = func() {
		updateMainContent(ui.NewHomeScreen())
		ui.GetGlobalState().SetCurrentView("home")
	}
	sidebar.OnWalletClicked = func() {
		updateMainContent(walletManager.NewWalletScreen())
		ui.GetGlobalState().SetCurrentView("wallet")
	}
	sidebar.OnAddressBookClicked = func() {
		updateMainContent(ui.NewAddressBookScreen())
		ui.GetGlobalState().SetCurrentView("addressbook")
	}
	sidebar.OnSwapClicked = func() {
		updateMainContent(ui.NewSwapScreen())
		ui.GetGlobalState().SetCurrentView("swap")
	}
	sidebar.OnBulkActionsClicked = func() {
		updateMainContent(ui.NewBulkActionsScreen())
		ui.GetGlobalState().SetCurrentView("bulkactions")
	}
	sidebar.OnCalypsoClicked = func() {
		updateMainContent(ui.NewCalypsoScreen(myWindow))
		ui.GetGlobalState().SetCurrentView("calypso")
	}
	sidebar.OnTxHistoryClicked = func() {
		updateMainContent(ui.NewTxHistoryScreen())
		ui.GetGlobalState().SetCurrentView("txhistory")
	}
	sidebar.OnUnruggableClicked = func() {
		updateMainContent(ui.NewUnruggableScreen(myWindow, myApp))
		ui.GetGlobalState().SetCurrentView("unruggable")
	}

	sidebar.OnWalletClicked() // Start with the home screen

	myWindow.ShowAndRun()
}
