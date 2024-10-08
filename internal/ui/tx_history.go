package ui

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type Transaction struct {
	Time        string `json:"time"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Signature   string `json:"signature"`
}

var (
	transactionCache      = make(map[string][]Transaction)
	transactionCacheMutex sync.RWMutex
	transactionCacheTime  = make(map[string]time.Time)
	cacheDuration         = 5 * time.Minute
)

func fetchTransactionHistory(walletAddress string) ([]Transaction, error) {
	transactionCacheMutex.RLock()
	if cachedTransactions, found := transactionCache[walletAddress]; found {
		if time.Since(transactionCacheTime[walletAddress]) < cacheDuration {
			transactionCacheMutex.RUnlock()
			return cachedTransactions, nil
		}
	}
	transactionCacheMutex.RUnlock()

	url := fmt.Sprintf("http://localhost:3000/api/solana/%s/transactions", walletAddress)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var transactions []Transaction
	err = json.Unmarshal(body, &transactions)
	if err != nil {
		return nil, err
	}

	transactionCacheMutex.Lock()
	transactionCache[walletAddress] = transactions
	transactionCacheTime[walletAddress] = time.Now()
	transactionCacheMutex.Unlock()

	return transactions, nil
}

func shortenHash(hash string) string {
	if len(hash) <= 12 {
		return hash
	}
	return fmt.Sprintf("%s...%s", hash[:6], hash[len(hash)-6:])
}

func shortenHashesInText(text string) string {
	hashRegex := regexp.MustCompile(`\b[a-fA-F0-9]{32,}\b`)
	return hashRegex.ReplaceAllStringFunc(text, shortenHash)
}

func NewTxHistoryScreen() fyne.CanvasObject {
	state := GetGlobalState()
	walletAddress := state.GetSelectedWallet()

	walletLabel := widget.NewLabel("No wallet selected")
	if walletAddress != "" {
		walletLabel.SetText(fmt.Sprintf("Loaded Wallet: %s", shortenHash(walletAddress)))
	}

	txContainer := container.NewVBox()

	// Function to update transactions
	updateTransactions := func() {
		if walletAddress == "" {
			txContainer.Objects = []fyne.CanvasObject{widget.NewLabel("No wallet selected")}
			txContainer.Refresh()
			return
		}

		transactions, err := fetchTransactionHistory(walletAddress)
		txContainer.Objects = nil // Clear previous transactions or loading label
		if err != nil {
			txContainer.Add(widget.NewLabel("Error fetching transactions"))
		} else {
			for _, tx := range transactions {
				txTime := widget.NewLabelWithStyle(tx.Time, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
				txDescription := widget.NewLabel(shortenHashesInText(tx.Description))
				txSignature := widget.NewLabelWithStyle(fmt.Sprintf("Signature: %s", shortenHash(tx.Signature)), fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})

				txCard := container.NewVBox(
					container.NewHBox(txTime, widget.NewLabel(fmt.Sprintf("[%s]", tx.Type))),
					txDescription,
					txSignature,
					widget.NewSeparator(),
				)

				txContainer.Add(txCard)
			}
		}
		txContainer.Refresh()
	}

	scrollContainer := container.NewVScroll(txContainer)
	scrollContainer.SetMinSize(fyne.NewSize(580, 700))

	// Main content layout
	content := container.NewVBox(
		widget.NewLabelWithStyle("Transaction History", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		walletLabel,
		scrollContainer,
	)

	// Fetch transactions initially
	updateTransactions()

	return content
}
