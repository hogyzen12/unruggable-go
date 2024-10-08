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
}

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
			wt.SetSelectedWallet(wallet)
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

func (wt *WalletTabs) SetSelectedWallet(walletID string) {
	wt.selectedWallet = walletID
	GetGlobalState().SetSelectedWallet(walletID) // Set the selected wallet in global state
	for id, tab := range wt.tabs {
		if id == walletID {
			tab.Importance = widget.HighImportance
		} else {
			tab.Importance = widget.MediumImportance
		}
		tab.Refresh()
	}
}

func (wt *WalletTabs) Container() *fyne.Container {
	return wt.container
}
