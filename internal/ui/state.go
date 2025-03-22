package ui

import (
	"sync"
)

// AppState holds the global application state.
type AppState struct {
	SelectedWallet string
	CurrentView    string
	RPCURL         string
	WalletBalances *WalletResponse // Pointer to allow nil checks
}

// Singleton pattern for global state with thread safety.
var (
	globalState     *AppState
	globalStateLock sync.Mutex
)

// GetGlobalState retrieves or initializes the global state.
func GetGlobalState() *AppState {
	globalStateLock.Lock()
	defer globalStateLock.Unlock()

	if globalState == nil {
		globalState = &AppState{
			SelectedWallet: "",
			CurrentView:    "",
			RPCURL:         "https://special-blue-fog.solana-mainnet.quiknode.pro/d009d548b4b9dd9f062a8124a868fb915937976c/",
			WalletBalances: nil,
		}
	}
	return globalState
}

// UpdateWalletBalances updates the balances in the global state.
func (s *AppState) UpdateWalletBalances(balances *WalletResponse) {
	globalStateLock.Lock()
	defer globalStateLock.Unlock()
	s.WalletBalances = balances
}

// GetWalletBalances retrieves the current balances.
func (s *AppState) GetWalletBalances() *WalletResponse {
	globalStateLock.Lock()
	defer globalStateLock.Unlock()
	return s.WalletBalances
}

// SetSelectedWallet updates the selected wallet and resets balances.
func (s *AppState) SetSelectedWallet(wallet string) {
	globalStateLock.Lock()
	defer globalStateLock.Unlock()
	s.SelectedWallet = wallet
	s.WalletBalances = nil // Reset balances until refreshed
}

// GetSelectedWallet returns the currently selected wallet.
func (s *AppState) GetSelectedWallet() string {
	globalStateLock.Lock()
	defer globalStateLock.Unlock()
	return s.SelectedWallet
}

// SetCurrentView updates the current view.
func (s *AppState) SetCurrentView(view string) {
	globalStateLock.Lock()
	defer globalStateLock.Unlock()
	s.CurrentView = view
}

// GetCurrentView returns the current view.
func (s *AppState) GetCurrentView() string {
	globalStateLock.Lock()
	defer globalStateLock.Unlock()
	return s.CurrentView
}
