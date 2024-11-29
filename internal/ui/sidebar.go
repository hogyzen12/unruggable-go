package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type Sidebar struct {
	widget.BaseWidget
	OnHomeClicked        func()
	OnSendClicked        func()
	OnWalletClicked      func()
	OnAddressBookClicked func()
	OnTxHistoryClicked   func()
	//OnSwapClicked        func()
	//OnBulkActionsClicked func()
	//OnCalypsoClicked     func()
	//OnUnruggableClicked  func()
	//OnKeygenClicked      func()
	//OnSigningClicked     func()
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
	sendBtn := widget.NewButton("Send", func() {
		if s.OnSendClicked != nil {
			s.OnSendClicked()
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
	txHistoryBtn := widget.NewButton("TX History", func() {
		if s.OnTxHistoryClicked != nil {
			s.OnTxHistoryClicked()
		}
	})
	//swapBtn := widget.NewButton("Swap", func() {
	//	if s.OnSwapClicked != nil {
	//		s.OnSwapClicked()
	//	}
	//})
	//bulkActionsBtn := widget.NewButton("Bulk Actions", func() {
	//	if s.OnBulkActionsClicked != nil {
	//		s.OnBulkActionsClicked()
	//	}
	//})
	//calypsoBtn := widget.NewButton("Calypso", func() {
	//	if s.OnCalypsoClicked != nil {
	//		s.OnCalypsoClicked()
	//	}
	//})
	//unruggableBtn := widget.NewButton("Unruggable", func() {
	//	if s.OnUnruggableClicked != nil {
	//		s.OnUnruggableClicked()
	//	}
	//})
	//keygenBtn := widget.NewButton("Keygen", func() {
	//	if s.OnKeygenClicked != nil {
	//		s.OnKeygenClicked()
	//	}
	//})
	//signingBtn := widget.NewButton("Signing", func() {
	//	if s.OnSigningClicked != nil {
	//		s.OnSigningClicked()
	//	}
	//})

	//content := container.NewVBox(homeBtn, sendBtn, walletBtn, addressBookBtn, txHistoryBtn, swapBtn, bulkActionsBtn, calypsoBtn, unruggableBtn, keygenBtn, signingBtn)
	content := container.NewVBox(homeBtn, sendBtn, walletBtn, addressBookBtn, txHistoryBtn)
	return widget.NewSimpleRenderer(content)
}
