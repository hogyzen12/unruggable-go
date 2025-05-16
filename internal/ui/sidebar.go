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
	OnTxInspectorClicked    func()
	OnMultisigCreateClicked func()
	OnMultisigInfoClicked   func()
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

	txInspectorBtn := widget.NewButton("Tx Inspector", func() {
		if s.OnTxInspectorClicked != nil {
			s.OnTxInspectorClicked()
		}
	})

	OnMultisigCreateClickedBtn := widget.NewButton("Squads Create", func() {
		if s.OnMultisigCreateClicked != nil {
			s.OnMultisigCreateClicked()
		}
	})

	infoBtn := widget.NewButton("Multisig Info", func() {
		if s.OnMultisigInfoClicked != nil {
			s.OnMultisigInfoClicked()
		}
	})

	content := container.NewVBox(
		homeBtn,
		sendBtn,
		walletBtn,
		calypsoBtn,
		conditionalBotBtn,
		hardwareSignBtn,
		txInspectorBtn,
		OnMultisigCreateClickedBtn,
		infoBtn)

	return widget.NewSimpleRenderer(content)
}
