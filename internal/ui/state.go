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