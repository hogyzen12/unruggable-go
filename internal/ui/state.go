package ui

import (
	"sync"
)

var (
	globalState     *AppState
	globalStateLock sync.Mutex
)

type AppState struct {
	SelectedWallet string
	CurrentView    string
}

func GetGlobalState() *AppState {
	globalStateLock.Lock()
	defer globalStateLock.Unlock()

	if globalState == nil {
		globalState = &AppState{}
	}
	return globalState
}

func (s *AppState) SetSelectedWallet(pubKey string) {
	globalStateLock.Lock()
	defer globalStateLock.Unlock()

	s.SelectedWallet = pubKey
}

func (s *AppState) GetSelectedWallet() string {
	globalStateLock.Lock()
	defer globalStateLock.Unlock()

	return s.SelectedWallet
}

func (s *AppState) SetCurrentView(view string) {
	globalStateLock.Lock()
	defer globalStateLock.Unlock()

	s.CurrentView = view
}

func (s *AppState) GetCurrentView() string {
	globalStateLock.Lock()
	defer globalStateLock.Unlock()

	return s.CurrentView
}
