package ui

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/gagliardetto/solana-go"
)

type WalletManager struct {
	window        fyne.Window
	walletList    *container.Scroll
	wallets       []string
	currentWallet *widget.Label
	walletTabs    *WalletTabs
	app           fyne.App
}

func NewWalletManager(window fyne.Window, walletTabs *WalletTabs, app fyne.App) *WalletManager {
	manager := &WalletManager{
		window:        window,
		wallets:       []string{},
		currentWallet: widget.NewLabel("No wallet selected"),
		walletTabs:    walletTabs,
		app:           app,
	}
	manager.loadSavedWallets()

	// Set up the onSwitch function for walletTabs
	walletTabs.onSwitch = func(walletID string) {
		manager.SetSelectedWallet(walletID)
	}

	return manager
}

func (m *WalletManager) GetWalletsDirectory() string {
	rootURI := m.app.Storage().RootURI()
	userDir := rootURI.Path()

	// Create the "wallets" subdirectory if it doesn't exist
	walletsDir := filepath.Join(userDir, "wallets")
	if _, err := os.Stat(walletsDir); os.IsNotExist(err) {
		os.MkdirAll(walletsDir, 0700)
	}

	return walletsDir
}

func (m *WalletManager) NewWalletScreen() fyne.CanvasObject {
	walletItems := container.NewVBox()

	for _, wallet := range m.wallets {
		walletButton := widget.NewButton(wallet[:8]+"...", func() {
			m.SetSelectedWallet(wallet)
			GetGlobalState().SetSelectedWallet(wallet)
		})
		walletItems.Add(walletButton)
	}

	m.walletList = container.NewVScroll(walletItems)
	m.walletList.SetMinSize(fyne.NewSize(200, 200))

	importEntry := widget.NewPasswordEntry()
	importEntry.SetPlaceHolder("Enter private key (Base58 or JSON Array)")

	importButton := widget.NewButton("Import Wallet", func() {
		m.importWallet(importEntry.Text)
		importEntry.SetText("") // Clear the entry for security
	})

	generateButton := widget.NewButton("Generate New Wallet", func() {
		m.generateWallet()
	})

	controls := container.NewVBox(
		widget.NewLabel("Wallet Management"),
		m.currentWallet,
		importEntry,
		importButton,
		generateButton,
	)

	return container.NewBorder(controls, nil, nil, nil, m.walletList)
}

func (m *WalletManager) loadSavedWallets() {
	walletsDir := m.GetWalletsDirectory()
	files, err := ioutil.ReadDir(walletsDir)
	if err != nil {
		if os.IsNotExist(err) {
			os.MkdirAll(walletsDir, 0700)
		} else {
			dialog.ShowError(fmt.Errorf("failed to read wallet directory: %v", err), m.window)
		}
		return
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".wallet") {
			m.wallets = append(m.wallets, strings.TrimSuffix(file.Name(), ".wallet"))
		}
	}

	if m.walletTabs != nil {
		m.walletTabs.Update(m.wallets)
	} else {
		fmt.Println("Warning: walletTabs is nil, skipping Update()")
	}
}

func (m *WalletManager) importWallet(privateKey string) {
	// Use the private key to derive the wallet
	wallet, err := solana.WalletFromPrivateKeyBase58(privateKey)
	if err != nil {
		dialog.ShowError(fmt.Errorf("invalid private key: %v", err), m.window)
		return
	}

	pubKey := wallet.PublicKey().String()

	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Enter password to encrypt wallet")

	passwordDialog := dialog.NewCustomConfirm("Encrypt Wallet", "Save", "Cancel", passwordEntry, func(encrypt bool) {
		if encrypt {
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

			m.wallets = append(m.wallets, pubKey)
			m.walletList.Refresh()
			m.walletTabs.Update(m.wallets) // Update tabs after importing a wallet
			dialog.ShowInformation("Wallet Imported", "Wallet imported and securely stored", m.window)
		}
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
		if encrypt {
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

			m.wallets = append(m.wallets, pubKey)
			m.walletList.Refresh()
			m.walletTabs.Update(m.wallets)
			dialog.ShowInformation("Wallet Generated", fmt.Sprintf("New wallet generated and securely stored. Public Key: %s", pubKey), m.window)
		}
	}, m.window)

	passwordDialog.Show()
}

func (m *WalletManager) SetSelectedWallet(walletID string) {
	m.currentWallet.SetText("Selected wallet: " + walletID)
	GetGlobalState().SetSelectedWallet(walletID)

	// Update the UI to reflect the selected wallet
	if m.walletTabs != nil {
		m.walletTabs.SetSelectedWallet(walletID)
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

	walletsDir := m.GetWalletsDirectory()
	filename := filepath.Join(walletsDir, pubKey+".wallet")

	if err := ioutil.WriteFile(filename, []byte(encryptedKey), 0600); err != nil {
		return err
	}

	// Add new wallet to list and update WalletTabs
	m.wallets = append(m.wallets, pubKey)
	if m.walletTabs != nil {
		m.walletTabs.Update(m.wallets)
	}

	return nil
}

func encrypt(data []byte, passphrase string) (string, error) {
	block, _ := aes.NewCipher([]byte(padKey(passphrase)))
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
