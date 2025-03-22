//go:build js && wasm
// +build js,wasm

package storage

import (
	"encoding/json"

	"fyne.io/fyne/v2"
)

// WalletStorage is the interface that abstracts wallet persistence.
type WalletStorage interface {
	SaveWallet(pubKey, encryptedKey string) error
	LoadWallets() (map[string]string, error)
}

// PrefWalletStorage implements WalletStorage for WASM using Preferences.
type PrefWalletStorage struct {
	app fyne.App
}

func NewWalletStorage(app fyne.App) WalletStorage {
	return &PrefWalletStorage{app: app}
}

const walletMapKey = "walletMap"

// SaveWallet saves the wallet (pubKey -> encryptedKey) to Preferences.
func (ps *PrefWalletStorage) SaveWallet(pubKey, encryptedKey string) error {
	prefs := ps.app.Preferences()
	// Get the current wallet map from preferences.
	wallets := make(map[string]string)
	stored := prefs.String(walletMapKey)
	if stored != "" {
		if err := json.Unmarshal([]byte(stored), &wallets); err != nil {
			return err
		}
	}
	// Update the map.
	wallets[pubKey] = encryptedKey
	data, err := json.Marshal(wallets)
	if err != nil {
		return err
	}
	prefs.SetString(walletMapKey, string(data))
	return nil
}

// LoadWallets retrieves the wallet map from Preferences.
func (ps *PrefWalletStorage) LoadWallets() (map[string]string, error) {
	wallets := make(map[string]string)
	stored := ps.app.Preferences().String(walletMapKey)
	if stored != "" {
		if err := json.Unmarshal([]byte(stored), &wallets); err != nil {
			return nil, err
		}
	}
	return wallets, nil
}
