package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/shopspring/decimal"
)

type TokenInfo struct {
	Address  string `json:"address"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
	PriceUSD float64 `json:"priceUSD"` // Store token price in USD if available
	Balance  json.RawMessage `json:"balance"` // Raw balance (can be string or number)
}

type AssetInfo struct {
	ID        string    `json:"id"`
	TokenInfo TokenInfo `json:"token_info"`
}

type AssetsResponse struct {
	Result struct {
		Items         []AssetInfo `json:"items"`
		NativeBalance struct {
			Lamports int64 `json:"lamports"`
		} `json:"nativeBalance"`
	} `json:"result"`
}

var (
	tokenList     []TokenInfo
	tokenListMu   sync.RWMutex
	tokenListTime time.Time
)

const tokenListURL = "https://tokens.jup.ag/tokens?tags=verified"
const tokenListCacheDuration = 1 * time.Hour
const RPC_ENDPOINT = "https://mainnet.helius-rpc.com/?api-key=2c0388dc-a082-4cc5-bad9-29437f3c0715"

// Function to fetch token prices
func fetchTokenList() error {
	tokenListMu.Lock()
	defer tokenListMu.Unlock()

	if time.Since(tokenListTime) < tokenListCacheDuration {
		return nil // Use cached list
	}

	resp, err := http.Get(tokenListURL)
	if err != nil {
		return fmt.Errorf("failed to fetch token list: %v", err)
	}
	defer resp.Body.Close()

	var tokens []TokenInfo
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return fmt.Errorf("failed to decode token list: %v", err)
	}

	tokenList = tokens
	tokenListTime = time.Now()
	return nil
}

// NewHomeScreen is the main screen for displaying wallet balances
func NewHomeScreen() fyne.CanvasObject {
	if err := fetchTokenList(); err != nil {
		fmt.Println("Warning: Failed to fetch token list:", err)
	}

	selectedWallet := GetGlobalState().GetSelectedWallet()
	walletLabel := widget.NewLabel("No wallet selected")
	if selectedWallet != "" {
		walletLabel.SetText(fmt.Sprintf("Loaded Wallet: %s", selectedWallet))
	}

	// Create a container for the balances
	balanceContainer := container.NewVBox()

	// Fetch balances and display
	updateBalances := func() {
		balances, err := getWalletBalances(selectedWallet)
		if err != nil {
			balanceContainer.Add(widget.NewLabel("Error fetching balances"))
			return
		}

		balanceContainer.Objects = nil // Clear previous balances

		// Sort tokens by symbol
		sortedTokens := make([]string, 0, len(balances))
		for token := range balances {
			sortedTokens = append(sortedTokens, token)
		}
		sort.Strings(sortedTokens)

		// Add balances to container with formatted USD values
		for _, token := range sortedTokens {
			balance := balances[token]

			// Get token info (including USD price)
			var priceUSD float64
			for _, t := range tokenList {
				if t.Symbol == token {
					priceUSD = t.PriceUSD
					break
				}
			}

			// Calculate USD value if price is available
			usdValue := balance.Mul(decimal.NewFromFloat(priceUSD))
			usdValueStr := ""
			if priceUSD > 0 {
				usdValueStr = fmt.Sprintf(" (~$%.2f)", usdValue)
			}

			// Display token balance and USD equivalent
			tokenLabel := widget.NewLabel(fmt.Sprintf("%s: %s%s", token, balance.String(), usdValueStr))

			// Add balance to the container
			balanceContainer.Add(tokenLabel)
		}
	}

	// Add update button and initial balance display
	updateButton := widget.NewButton("Update Balances", updateBalances)

	// Main content layout
	content := container.NewVBox(
		widget.NewLabel("Welcome to Unruggable"),
		walletLabel,
		widget.NewLabel("Holdings:"),
		balanceContainer,
		updateButton,
	)

	// Fetch balances initially
	updateBalances()

	return content
}

func getWalletBalances(walletAddress string) (map[string]decimal.Decimal, error) {
	requestBody, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "my-id",
		"method":  "getAssetsByOwner",
		"params": map[string]interface{}{
			"ownerAddress": walletAddress,
			"page":         1,
			"limit":        1000,
			"displayOptions": map[string]bool{
				"showFungible":     true,
				"showNativeBalance": true,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	resp, err := http.Post(RPC_ENDPOINT, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	var response AssetsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	balances := make(map[string]decimal.Decimal)

	for _, item := range response.Result.Items {
		var balance decimal.Decimal
		var balanceStr string
		if err := json.Unmarshal(item.TokenInfo.Balance, &balanceStr); err == nil {
			balance, err = decimal.NewFromString(balanceStr)
			if err != nil {
				continue
			}
		} else {
			var balanceNum float64
			if err := json.Unmarshal(item.TokenInfo.Balance, &balanceNum); err == nil {
				balance = decimal.NewFromFloat(balanceNum)
			} else {
				continue
			}
		}
		decimals := decimal.New(1, int32(item.TokenInfo.Decimals))
		balances[item.TokenInfo.Symbol] = balance.Div(decimals)
	}

	// Add SOL balance
	solBalance := decimal.NewFromInt(response.Result.NativeBalance.Lamports)
	balances["SOL"] = solBalance.Div(decimal.New(1, 9)) // 9 decimals for SOL

	return balances, nil
}
