package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type Sidebar struct {
	widget.BaseWidget
	OnHomeClicked       func()
	OnWalletClicked     func()
	OnAddressBookClicked func()
	OnSwapClicked       func()
	OnBulkActionsClicked func()
	OnCalypsoClicked    func()
	OnTxHistoryClicked  func()
	OnUnruggableClicked func()
}

func NewSidebar() *Sidebar {
	s := &Sidebar{}
	s.ExtendBaseWidget(s)
	return s
}

func (s *Sidebar) CreateRenderer() fyne.WidgetRenderer {
	homeBtn := widget.NewButton("Home", func() {
		if s.OnHomeClicked != nil {
			s.OnHomeClicked()
		}
	})
	walletBtn := widget.NewButton("Wallet", func() {
		if s.OnWalletClicked != nil {
			s.OnWalletClicked()
		}
	})
	addressBookBtn := widget.NewButton("Address Book", func() {
		if s.OnAddressBookClicked != nil {
			s.OnAddressBookClicked()
		}
	})
	swapBtn := widget.NewButton("Swap", func() {
		if s.OnSwapClicked != nil {
			s.OnSwapClicked()
		}
	})
	bulkActionsBtn := widget.NewButton("Bulk Actions", func() {
		if s.OnBulkActionsClicked != nil {
			s.OnBulkActionsClicked()
		}
	})
	calypsoBtn := widget.NewButton("Calypso", func() {
		if s.OnCalypsoClicked != nil {
			s.OnCalypsoClicked()
		}
	})
	txHistoryBtn := widget.NewButton("TX History", func() {
		if s.OnTxHistoryClicked != nil {
			s.OnTxHistoryClicked()
		}
	})
	unruggableBtn := widget.NewButton("Unruggable", func() {
		if s.OnUnruggableClicked != nil {
			s.OnUnruggableClicked()
		}
	})

	content := container.NewVBox(homeBtn, walletBtn, addressBookBtn, swapBtn, bulkActionsBtn, calypsoBtn, txHistoryBtn, unruggableBtn)
	return widget.NewSimpleRenderer(content)
}