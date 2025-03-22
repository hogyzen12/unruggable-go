package ui

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/gagliardetto/solana-go"

	"unruggable-go/internal/storage"
)

type WalletManager struct {
	window        fyne.Window
	walletList    *container.Scroll
	wallets       []string
	currentWallet *widget.Label
	walletTabs    *WalletTabs
	app           fyne.App
	storage       storage.WalletStorage
}

func NewWalletManager(window fyne.Window, walletTabs *WalletTabs, app fyne.App) *WalletManager {
	manager := &WalletManager{
		window:        window,
		wallets:       []string{},
		currentWallet: widget.NewLabel("No wallet selected"),
		walletTabs:    walletTabs,
		app:           app,
		// Create the appropriate WalletStorage based on build tags.
		storage: storage.NewWalletStorage(app),
	}

	// Load wallets immediately
	manager.loadSavedWallets()

	// Set up the onSwitch function for walletTabs
	walletTabs.onSwitch = func(walletID string) {
		manager.SetSelectedWallet(walletID)
	}

	// Check if there's already a selected wallet in global state
	if selectedWallet := GetGlobalState().GetSelectedWallet(); selectedWallet != "" {
		// Verify that the selected wallet exists in our loaded wallets
		for _, wallet := range manager.wallets {
			if wallet == selectedWallet {
				manager.SetSelectedWallet(selectedWallet)
				break
			}
		}
	} else if len(manager.wallets) > 0 {
		// If no wallet is selected but wallets exist, select the first one
		manager.SetSelectedWallet(manager.wallets[0])
	}

	return manager
}

// NewWalletScreen creates the UI for wallet management.
func (m *WalletManager) NewWalletScreen() fyne.CanvasObject {
	// Create a refresh button to reload wallets
	refreshButton := widget.NewButton("Refresh Wallets", func() {
		m.loadSavedWallets()
	})

	walletItems := container.NewVBox()

	// Create a button for each wallet.
	for _, wallet := range m.wallets {
		walletButton := widget.NewButton(wallet[:8]+"...", func(wlt string) func() {
			return func() {
				m.SetSelectedWallet(wlt)
			}
		}(wallet))

		// Highlight the currently selected wallet
		if wallet == GetGlobalState().GetSelectedWallet() {
			walletButton.Importance = widget.HighImportance
		}

		walletItems.Add(walletButton)
	}

	m.walletList = container.NewVScroll(walletItems)
	m.walletList.SetMinSize(fyne.NewSize(200, 200))

	importEntry := widget.NewPasswordEntry()
	importEntry.SetPlaceHolder("Enter private key (Base58 or JSON Array)")

	importButton := widget.NewButton("Import Wallet", func() {
		m.importWallet(importEntry.Text)
		importEntry.SetText("") // Clear for security
	})

	generateButton := widget.NewButton("Generate New Wallet", func() {
		m.generateWallet()
	})

	controls := container.NewVBox(
		widget.NewLabel("Wallet Management"),
		m.currentWallet,
		refreshButton,
		container.NewHBox(importEntry, importButton),
		generateButton,
	)

	return container.NewBorder(controls, nil, nil, nil, m.walletList)
}

func (m *WalletManager) loadSavedWallets() {
	walletMap, err := m.storage.LoadWallets()
	if err != nil {
		dialog.ShowError(fmt.Errorf("failed to load wallets: %v", err), m.window)
		return
	}

	// Clear existing wallets list
	m.wallets = []string{}

	// Add wallets from the storage
	for pubKey := range walletMap {
		m.wallets = append(m.wallets, pubKey)
	}

	// Sort wallets for consistent display
	sort.Strings(m.wallets)

	// Update UI
	if m.walletTabs != nil {
		m.walletTabs.Update(m.wallets)
	}

	// Update wallet list if it exists
	if m.walletList != nil {
		m.walletList.Content = m.createWalletItemsList()
		m.walletList.Refresh()
	}

	// Check if the currently selected wallet still exists
	currentSelection := GetGlobalState().GetSelectedWallet()
	walletExists := false

	for _, wallet := range m.wallets {
		if wallet == currentSelection {
			walletExists = true
			break
		}
	}

	// If the selected wallet no longer exists, clear it or select a different one
	if !walletExists {
		if len(m.wallets) > 0 {
			m.SetSelectedWallet(m.wallets[0])
		} else {
			m.currentWallet.SetText("No wallet selected")
			GetGlobalState().SetSelectedWallet("")
		}
	}
}

// Helper method to create the wallet items list
func (m *WalletManager) createWalletItemsList() fyne.CanvasObject {
	walletItems := container.NewVBox()

	selectedWallet := GetGlobalState().GetSelectedWallet()

	for _, wallet := range m.wallets {
		walletButton := widget.NewButton(formatWalletName(wallet), func(wlt string) func() {
			return func() {
				m.SetSelectedWallet(wlt)
			}
		}(wallet))

		// Highlight the currently selected wallet
		if wallet == selectedWallet {
			walletButton.Importance = widget.HighImportance
		}

		walletItems.Add(walletButton)
	}

	if len(m.wallets) == 0 {
		walletItems.Add(widget.NewLabel("No wallets found. Import or generate a wallet."))
	}

	return walletItems
}

func (m *WalletManager) importWallet(privateKey string) {
	if privateKey == "" {
		dialog.ShowError(fmt.Errorf("please enter a private key"), m.window)
		return
	}

	wallet, err := solana.WalletFromPrivateKeyBase58(privateKey)
	if err != nil {
		dialog.ShowError(fmt.Errorf("invalid private key: %v", err), m.window)
		return
	}
	pubKey := wallet.PublicKey().String()

	// Check if wallet already exists
	walletMap, _ := m.storage.LoadWallets()
	if _, exists := walletMap[pubKey]; exists {
		dialog.ShowInformation("Wallet Exists", "This wallet is already imported. Selecting it now.", m.window)
		m.SetSelectedWallet(pubKey)
		return
	}

	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Enter password to encrypt wallet")

	passwordDialog := dialog.NewCustomConfirm("Encrypt Wallet", "Save", "Cancel", passwordEntry, func(encrypt bool) {
		if !encrypt {
			return
		}

		password := passwordEntry.Text
		if password == "" {
			dialog.ShowError(fmt.Errorf("password cannot be empty"), m.window)
			return
		}

		err := m.saveEncryptedWallet(pubKey, privateKey, password)
		if err != nil {
			dialog.ShowError(fmt.Errorf("failed to save wallet: %v", err), m.window)
			return
		}

		// Reload wallets to include the new one
		m.loadSavedWallets()

		// Select the newly imported wallet
		m.SetSelectedWallet(pubKey)

		dialog.ShowInformation("Wallet Imported", "Wallet imported and securely stored", m.window)
	}, m.window)

	passwordDialog.Show()
}

func (m *WalletManager) generateWallet() {
	wallet := solana.NewWallet()
	pubKey := wallet.PublicKey().String()
	privateKey := wallet.PrivateKey.String()

	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Enter password to encrypt wallet")

	passwordDialog := dialog.NewCustomConfirm("Encrypt Wallet", "Save", "Cancel", passwordEntry, func(encrypt bool) {
		if !encrypt {
			return
		}

		password := passwordEntry.Text
		if password == "" {
			dialog.ShowError(fmt.Errorf("password cannot be empty"), m.window)
			return
		}

		err := m.saveEncryptedWallet(pubKey, privateKey, password)
		if err != nil {
			dialog.ShowError(fmt.Errorf("failed to save wallet: %v", err), m.window)
			return
		}

		// Reload wallets to include the new one
		m.loadSavedWallets()

		// Select the newly generated wallet
		m.SetSelectedWallet(pubKey)

		dialog.ShowInformation(
			"Wallet Generated",
			fmt.Sprintf("New wallet generated and securely stored.\nPublic Key: %s", shortenAddress(pubKey)),
			m.window,
		)
	}, m.window)

	passwordDialog.Show()
}

func (m *WalletManager) SetSelectedWallet(walletID string) {
	// Set the display text
	m.currentWallet.SetText("Selected wallet: " + shortenAddress(walletID))

	// Update global state
	GetGlobalState().SetSelectedWallet(walletID)

	// Update wallet tabs UI if available
	if m.walletTabs != nil {
		m.walletTabs.SetSelectedWallet(walletID)
	}

	// Update the wallet list to highlight the selected wallet
	if m.walletList != nil {
		m.walletList.Content = m.createWalletItemsList()
		m.walletList.Refresh()
	}
}

func (m *WalletManager) GetWallets() []string {
	return m.wallets
}

func (m *WalletManager) saveEncryptedWallet(pubKey, privateKey, password string) error {
	encryptedKey, err := encrypt([]byte(privateKey), password)
	if err != nil {
		return err
	}

	// Save via our storage abstraction.
	if err := m.storage.SaveWallet(pubKey, encryptedKey); err != nil {
		return err
	}

	return nil
}

// Helper function to format wallet name (shortened address)
func formatWalletName(address string) string {
	return shortenAddress(address)
}

// encrypt encrypts data using AES-GCM.
func encrypt(data []byte, passphrase string) (string, error) {
	block, err := aes.NewCipher([]byte(padKey(passphrase)))
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return hex.EncodeToString(ciphertext), nil
}
