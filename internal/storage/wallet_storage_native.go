//go:build !js
// +build !js

package storage

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
)

// WalletStorage is the interface that abstracts wallet persistence.
type WalletStorage interface {
	SaveWallet(pubKey, encryptedKey string) error
	LoadWallets() (map[string]string, error)
}

// FileWalletStorage implements WalletStorage for native builds.
type FileWalletStorage struct {
	app fyne.App
}

func NewWalletStorage(app fyne.App) WalletStorage {
	return &FileWalletStorage{app: app}
}

// walletsDir returns the wallets directory in the app’s storage root.
func (fs *FileWalletStorage) walletsDir() string {
	rootURI := fs.app.Storage().RootURI()
	userDir := rootURI.Path()
	return filepath.Join(userDir, "wallets")
}

func (fs *FileWalletStorage) SaveWallet(pubKey, encryptedKey string) error {
	walletsDir := fs.walletsDir()
	if _, err := os.Stat(walletsDir); os.IsNotExist(err) {
		if err := os.MkdirAll(walletsDir, 0700); err != nil {
			return err
		}
	}
	filename := filepath.Join(walletsDir, pubKey+".wallet")
	return ioutil.WriteFile(filename, []byte(encryptedKey), 0600)
}

func (fs *FileWalletStorage) LoadWallets() (map[string]string, error) {
	wallets := make(map[string]string)
	walletsDir := fs.walletsDir()
	files, err := ioutil.ReadDir(walletsDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Create directory and return an empty map.
			if err := os.MkdirAll(walletsDir, 0700); err != nil {
				return nil, err
			}
			return wallets, nil
		}
		return nil, err
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".wallet" {
			content, err := ioutil.ReadFile(filepath.Join(walletsDir, file.Name()))
			if err != nil {
				continue // Optionally log error
			}
			pubKey := file.Name()[:len(file.Name())-len(".wallet")]
			wallets[pubKey] = string(content)
		}
	}
	return wallets, nil
}
