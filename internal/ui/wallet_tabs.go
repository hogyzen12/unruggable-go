package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type WalletTabs struct {
	container *fyne.Container
	tabs      []*widget.Button
	onSwitch  func(string)
}

func NewWalletTabs(onSwitch func(string)) *WalletTabs {
	wt := &WalletTabs{
		container: container.NewHBox(),
		onSwitch:  onSwitch,
	}
	return wt
}

func (wt *WalletTabs) Update(wallets []string) {
	wt.container.RemoveAll()
	wt.tabs = nil

	for _, wallet := range wallets {
		wallet := wallet // Capture for closure
		tab := widget.NewButton(wallet[:8]+"...", func() {
			wt.onSwitch(wallet)
		})
		wt.tabs = append(wt.tabs, tab)
		wt.container.Add(tab)
	}
}

func (wt *WalletTabs) Container() *fyne.Container {
	return wt.container
}