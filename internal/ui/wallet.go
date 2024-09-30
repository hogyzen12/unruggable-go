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

const walletStorageDir = "./wallets"

type WalletManager struct {
	window        fyne.Window
	walletList    *widget.List
	wallets       []string
	currentWallet *widget.Label
	walletTabs    *WalletTabs
}

func NewWalletManager(window fyne.Window, walletTabs *WalletTabs) *WalletManager {
	manager := &WalletManager{
		window:        window,
		wallets:       []string{},
		currentWallet: widget.NewLabel("No wallet selected"),
		walletTabs:    walletTabs,
	}
	manager.loadSavedWallets()
	return manager
}

func (m *WalletManager) NewWalletScreen() fyne.CanvasObject {
	m.walletList = widget.NewList(
		func() int { return len(m.wallets) },
		func() fyne.CanvasObject { return widget.NewLabel("template") },
		func(id widget.ListItemID, item fyne.CanvasObject) {
			item.(*widget.Label).SetText(m.wallets[id][:8] + "...")
		},
	)

	m.walletList.OnSelected = func(id widget.ListItemID) {
		selectedWallet := m.wallets[id]
		m.currentWallet.SetText("Selected wallet: " + selectedWallet)
		GetGlobalState().SetSelectedWallet(selectedWallet)
		m.walletTabs.Update(m.wallets) // Update tabs when a wallet is selected
	}

	importEntry := widget.NewPasswordEntry()
	importEntry.SetPlaceHolder("Enter private key (base58)")

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

func (m *WalletManager) GetWallets() []string {
	return m.wallets
}

func (m *WalletManager) loadSavedWallets() {
	files, err := ioutil.ReadDir(walletStorageDir)
	if err != nil {
		if os.IsNotExist(err) {
			os.MkdirAll(walletStorageDir, 0700)
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
	m.walletTabs.Update(m.wallets) // Update tabs after loading wallets
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
			m.walletTabs.Update(m.wallets) // Update tabs after generating a wallet
			dialog.ShowInformation("Wallet Generated", fmt.Sprintf("New wallet generated and securely stored. Public Key: %s", pubKey), m.window)
		}
	}, m.window)

	passwordDialog.Show()
}

func (m *WalletManager) saveEncryptedWallet(pubKey, privateKey, password string) error {
	encryptedKey, err := encrypt([]byte(privateKey), password)
	if err != nil {
		return err
	}

	filename := filepath.Join(walletStorageDir, pubKey+".wallet")
	return ioutil.WriteFile(filename, []byte(encryptedKey), 0600)
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

func padKey(key string) string {
	for len(key) < 32 {
		key += key
	}
	return key[:32]
}