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
	window         fyne.Window // Add this
	app            fyne.App    // Add this
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

func (wt *WalletTabs) Update(wallets []string) {
	if wt.container == nil {
		wt.container = container.NewHBox()
	}

	wt.container.RemoveAll()
	wt.tabs = make(map[string]*widget.Button)

	for _, wallet := range wallets {
		wallet := wallet // Capture for closure
		tab := widget.NewButton(wallet[:8]+"...", func() {
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
	if wt.selectedWallet != "" {
		wt.SetSelectedWallet(wt.selectedWallet)
	}
}

func (wt *WalletTabs) refreshCurrentScreen(view string) {
	if wt.mainContent == nil {
		return
	}

	wt.mainContent.RemoveAll()

	// Create new instance of current screen with updated wallet
	var newContent fyne.CanvasObject
	switch view {
	case "home":
		newContent = NewHomeScreen()
	case "send":
		newContent = NewSendScreen(wt.window, wt.app)
	case "addressbook":
		newContent = NewAddressBookScreen()
	case "txhistory":
		newContent = NewTxHistoryScreen()
	// Add other cases for different screens
	default:
		return
	}

	wt.mainContent.Add(newContent)
	wt.mainContent.Refresh()
}

// Add these to wallet_tabs.go
func (wt *WalletTabs) SetMainContent(content *fyne.Container) {
	wt.mainContent = content
}

func (wt *WalletTabs) SetWindow(window fyne.Window) {
	wt.window = window
}

func (wt *WalletTabs) SetApp(app fyne.App) {
	wt.app = app
}

func (wt *WalletTabs) SetSelectedWallet(walletID string) {
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
}

func (wt *WalletTabs) VerifyGlobalState() bool {
	return GetGlobalState().GetSelectedWallet() == wt.selectedWallet
}

func (wt *WalletTabs) Container() *fyne.Container {
	return wt.container
}
