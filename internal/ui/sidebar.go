package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type Sidebar struct {
	widget.BaseWidget
	OnHomeClicked           func()
	OnSendClicked           func()
	OnWalletClicked         func()
	OnAddressBookClicked    func()
	OnTxHistoryClicked      func()
	OnCalypsoClicked        func()
	OnConditionalBotClicked func() // New field for Conditional Bot
	OnHardwareSignClicked   func() // New field for Conditional Bot
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
	calypsoBtn := widget.NewButton("Calypso", func() {
		if s.OnCalypsoClicked != nil {
			s.OnCalypsoClicked()
		}
	})
	conditionalBotBtn := widget.NewButton("Conditional Bot", func() {
		if s.OnConditionalBotClicked != nil {
			s.OnConditionalBotClicked()
		}
	})
	hardwareSignBtn := widget.NewButton("Hardware Sign", func() {
		if s.OnHardwareSignClicked != nil {
			s.OnHardwareSignClicked()
		}
	})

	content := container.NewVBox(
		homeBtn,
		sendBtn,
		walletBtn,
		calypsoBtn,
		conditionalBotBtn,
		hardwareSignBtn)

	return widget.NewSimpleRenderer(content)
}
