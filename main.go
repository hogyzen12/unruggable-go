package main

import (
	"fmt"
	"unruggable-go/internal/ui"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func main() {
	myApp := app.NewWithID("com.unruggable.app")
	myWindow := myApp.NewWindow("Unruggable")

	// Create wallet tabs with state synchronization
	walletTabs := ui.NewWalletTabs(func(walletID string) {
		fmt.Println("Switched to wallet:", walletID)
	})

	// Create the main content container
	mainContent := container.NewStack()

	// Set references for wallet tabs
	walletTabs.SetMainContent(mainContent)
	walletTabs.SetWindow(myWindow)
	walletTabs.SetApp(myApp)

	// Initialize wallet manager
	walletManager := ui.NewWalletManager(myWindow, walletTabs, myApp)

	// Status bar for application-wide messages
	statusBar := widget.NewLabel("")
	statusBar.Alignment = fyne.TextAlignCenter

	// Create the layout with wallet tabs at the bottom
	content := container.NewBorder(
		nil, // Top
		container.NewVBox( // Bottom
			walletTabs.Container(),
			container.NewHBox( // Status bar with padding
				widget.NewLabel(""), // Left padding
				statusBar,
				widget.NewLabel(""), // Right padding
			),
		),
		nil,         // Left
		nil,         // Right
		mainContent, // Center
	)

	// Create sidebar
	sidebar := ui.NewSidebar()

	// Create the split layout
	split := container.NewHSplit(sidebar, content)
	split.SetOffset(0.2) // 20% width for sidebar

	// Set the window content
	myWindow.SetContent(split)
	myWindow.Resize(fyne.NewSize(1000, 700))

	// Function to update main content
	updateMainContent := func(newContent fyne.CanvasObject) {
		mainContent.RemoveAll()
		mainContent.Add(newContent)
	}

	// Setup sidebar navigation
	sidebar.OnHomeClicked = func() {
		updateMainContent(ui.NewHomeScreen())
		ui.GetGlobalState().SetCurrentView("home")
	}

	sidebar.OnSendClicked = func() {
		// Check if a wallet is selected
		if walletID := ui.GetGlobalState().GetSelectedWallet(); walletID == "" {
			statusBar.SetText("Please select a wallet first")
			updateMainContent(walletManager.NewWalletScreen())
			ui.GetGlobalState().SetCurrentView("wallet")
			return
		}

		updateMainContent(ui.NewSendScreen(myWindow, myApp))
		ui.GetGlobalState().SetCurrentView("send")
		statusBar.SetText("")
	}

	sidebar.OnWalletClicked = func() {
		updateMainContent(walletManager.NewWalletScreen())
		ui.GetGlobalState().SetCurrentView("wallet")
		statusBar.SetText("")
	}

	sidebar.OnCalypsoClicked = func() {
		// Check if a wallet is selected
		if walletID := ui.GetGlobalState().GetSelectedWallet(); walletID == "" {
			statusBar.SetText("Please select a wallet first")
			updateMainContent(walletManager.NewWalletScreen())
			ui.GetGlobalState().SetCurrentView("wallet")
			return
		}

		updateMainContent(ui.NewCalypsoScreen(myWindow, myApp))
		ui.GetGlobalState().SetCurrentView("calypso")
		statusBar.SetText("")
	}

	sidebar.OnConditionalBotClicked = func() {
		// Check if a wallet is selected
		if walletID := ui.GetGlobalState().GetSelectedWallet(); walletID == "" {
			statusBar.SetText("Please select a wallet first")
			updateMainContent(walletManager.NewWalletScreen())
			ui.GetGlobalState().SetCurrentView("wallet")
			return
		}

		updateMainContent(ui.NewConditionalBotScreen(myWindow, myApp))
		ui.GetGlobalState().SetCurrentView("conditionalbot")
		statusBar.SetText("")
	}

	sidebar.OnHardwareSignClicked = func() {
		updateMainContent(ui.NewSignScreen())
		ui.GetGlobalState().SetCurrentView("hardware")
		statusBar.SetText("")
	}

	// Add transaction inspector function
	sidebar.OnTxInspectorClicked = func() {
		updateMainContent(ui.NewTransactionInspectorScreen(myWindow, myApp))
		ui.GetGlobalState().SetCurrentView("txinspector")
		statusBar.SetText("")
	}

	sidebar.OnMultisigCreateClicked = func() {
		updateMainContent(ui.NewMultisigCreateScreen(myWindow))
		ui.GetGlobalState().SetCurrentView("multisigcreate")
		statusBar.SetText("")
	}

	sidebar.OnMultisigInfoClicked = func() {
		updateMainContent(ui.NewMultisigInfoScreen(myWindow))
		ui.GetGlobalState().SetCurrentView("multisiginfo")
		statusBar.SetText("")
	}

	// Start with the wallet screen as the default view
	sidebar.OnWalletClicked()

	// Show the window and start the application event loop
	myWindow.ShowAndRun()
}
