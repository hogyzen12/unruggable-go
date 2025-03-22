package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type WalletTabs struct {
	container      *fyne.Container
	tabs           map[string]*widget.Button
	onSwitch       func(string)
	selectedWallet string
	mainContent    *fyne.Container
	window         fyne.Window
	app            fyne.App
}

// Update constructor to initialize these fields
func NewWalletTabs(onSwitch func(string)) *WalletTabs {
	wt := &WalletTabs{
		container: container.NewHBox(),
		tabs:      make(map[string]*widget.Button),
		onSwitch:  onSwitch,
	}
	return wt
}

// Update refreshes the wallet tabs with the current list of wallets
func (wt *WalletTabs) Update(wallets []string) {
	if wt.container == nil {
		wt.container = container.NewHBox()
	}

	// Clear existing tabs
	wt.container.RemoveAll()
	wt.tabs = make(map[string]*widget.Button)

	// Check if we have wallets to display
	if len(wallets) == 0 {
		noWalletLabel := widget.NewLabel("No wallets available")
		wt.container.Add(noWalletLabel)
		return
	}

	// Add tabs for each wallet
	for _, wallet := range wallets {
		wallet := wallet // Capture for closure
		displayName := formatWalletDisplay(wallet)

		tab := widget.NewButton(displayName, func() {
			// Update global state immediately
			GetGlobalState().SetSelectedWallet(wallet)

			// Update visual state
			wt.SetSelectedWallet(wallet)

			// Get current view and refresh screen
			currentView := GetGlobalState().GetCurrentView()
			wt.refreshCurrentScreen(currentView)

			// Notify any listeners
			if wt.onSwitch != nil {
				wt.onSwitch(wallet)
			}
		})

		wt.tabs[wallet] = tab
		wt.container.Add(tab)
	}

	// If there's a selected wallet, highlight it
	currentSelection := GetGlobalState().GetSelectedWallet()
	if currentSelection != "" {
		wt.SetSelectedWallet(currentSelection)
	} else if wt.selectedWallet != "" {
		wt.SetSelectedWallet(wt.selectedWallet)
	} else if len(wallets) > 0 {
		// If no selection exists, select the first wallet
		wt.SetSelectedWallet(wallets[0])
	}

	wt.container.Refresh()
}

// Format wallet address for display
func formatWalletDisplay(wallet string) string {
	if len(wallet) <= 10 {
		return wallet
	}
	return wallet[:6] + "..." + wallet[len(wallet)-4:]
}

// Refresh the current screen with updated wallet information
func (wt *WalletTabs) refreshCurrentScreen(view string) {
	if wt.mainContent == nil {
		return
	}

	// Save the current view to global state
	GetGlobalState().SetCurrentView(view)

	// Remove all content and rebuild
	wt.mainContent.RemoveAll()

	// Create new instance of current screen with updated wallet
	var newContent fyne.CanvasObject
	switch view {
	case "home":
		newContent = NewHomeScreen()
	case "send":
		newContent = NewSendScreen(wt.window, wt.app)
	case "calypso":
		newContent = NewCalypsoScreen(wt.window, wt.app)
	case "conditionalbot":
		newContent = NewConditionalBotScreen(wt.window, wt.app)
	case "wallet":
		// Keep the wallet screen available as a fallback option
		return
	default:
		// Default to home screen if unknown view
		newContent = NewHomeScreen()
		GetGlobalState().SetCurrentView("home")
	}

	wt.mainContent.Add(newContent)
	wt.mainContent.Refresh()
}

// SetMainContent connects the wallet tabs to the main content area
func (wt *WalletTabs) SetMainContent(content *fyne.Container) {
	wt.mainContent = content
}

// SetWindow sets the window reference
func (wt *WalletTabs) SetWindow(window fyne.Window) {
	wt.window = window
}

// SetApp sets the app reference
func (wt *WalletTabs) SetApp(app fyne.App) {
	wt.app = app
}

// SetSelectedWallet updates the selected wallet in both local and global state
func (wt *WalletTabs) SetSelectedWallet(walletID string) {
	if walletID == "" {
		return
	}

	// Set the selected wallet in our local state
	wt.selectedWallet = walletID

	// Ensure global state is updated
	GetGlobalState().SetSelectedWallet(walletID)

	// Update visual state of tabs
	for id, tab := range wt.tabs {
		if id == walletID {
			tab.Importance = widget.HighImportance
		} else {
			tab.Importance = widget.MediumImportance
		}
		tab.Refresh()
	}

	// Refresh current screen if it exists and we have a view set
	if wt.mainContent != nil && GetGlobalState().GetCurrentView() != "" {
		wt.refreshCurrentScreen(GetGlobalState().GetCurrentView())
	}
}

// VerifyGlobalState checks if the local and global state are in sync
func (wt *WalletTabs) VerifyGlobalState() bool {
	return GetGlobalState().GetSelectedWallet() == wt.selectedWallet
}

// Container returns the underlying container
func (wt *WalletTabs) Container() *fyne.Container {
	return wt.container
}
