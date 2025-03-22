package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// Token list caching variables
var (
	tokenList     []Token
	tokenListMu   sync.RWMutex
	tokenListTime time.Time
)

const (
	tokenListURL           = "https://api.jup.ag/tokens/v1/tagged/verified"
	tokenListCacheDuration = 1 * time.Hour
	jupiterPriceAPIURL     = "https://api.jup.ag/price/v2"
	pythPriceAPIURL        = "https://hermes.pyth.network/v2/updates/price/latest"
)

// TokenPriceIDs maps token symbols to Pyth price feed IDs
var TokenPriceIDs = map[string]string{
	"SOL": "ef0d8b6fda2ceba41da15d4095d1da392a0d2f8ed0c6c7bc0f4cfac8c280b56d",
	"JUP": "0a0408d619e9380abad35060f9192039ed5042fa6f82301d0e48bb52be830996",
}

// RPCRequest defines the structure for Solana RPC requests
type RPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      string        `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

// RPCResponseGetBalance defines the response for getBalance
type RPCResponseGetBalance struct {
	Result struct {
		Context struct {
			Slot int64 `json:"slot"`
		} `json:"context"`
		Value uint64 `json:"value"` // Lamports
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// TokenAccount represents an SPL token account from getTokenAccountsByOwner
type TokenAccount struct {
	Pubkey  string `json:"pubkey"`
	Account struct {
		Data struct {
			Parsed struct {
				Info struct {
					Mint        string `json:"mint"`
					TokenAmount struct {
						Amount   string  `json:"amount"`
						Decimals int     `json:"decimals"`
						UIAmount float64 `json:"uiAmount"`
					} `json:"tokenAmount"`
				} `json:"info"`
				Type string `json:"type"`
			} `json:"parsed"`
			Program string `json:"program"`
		} `json:"data"`
	} `json:"account"`
}

// RPCResponseGetTokenAccounts defines the response for getTokenAccountsByOwner
type RPCResponseGetTokenAccounts struct {
	Result struct {
		Context struct {
			Slot int64 `json:"slot"`
		} `json:"context"`
		Value []TokenAccount `json:"value"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// JupiterPriceResponse defines the Jupiter price API response
type JupiterPriceResponse struct {
	Data map[string]struct {
		ID    string `json:"id"`
		Type  string `json:"type"`
		Price string `json:"price"` // Changed to string to match API response
	} `json:"data"`
	TimeTaken float64 `json:"timeTaken"`
}

// PythPriceItem defines a single price item from Pyth
type PythPriceItem struct {
	ID    string `json:"id"`
	Price struct {
		Price string `json:"price"`
		Expo  int32  `json:"expo"`
	} `json:"price"`
}

// PythPriceResponse defines the Pyth price API response
type PythPriceResponse struct {
	Parsed []PythPriceItem `json:"parsed"`
}

// getTokenList fetches or returns the cached Jupiter token list
func getTokenList() ([]Token, error) {
	tokenListMu.RLock()
	if time.Since(tokenListTime) < tokenListCacheDuration && len(tokenList) > 0 {
		defer tokenListMu.RUnlock()
		return tokenList, nil
	}
	tokenListMu.RUnlock()

	resp, err := http.Get(tokenListURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch token list: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token list response: %v", err)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("token list response is empty")
	}

	var tokens []Token
	if err := json.Unmarshal(body, &tokens); err != nil {
		return nil, fmt.Errorf("failed to decode token list: %v", err)
	}

	fmt.Printf("Fetched token list: %d tokens\n", len(tokens))
	tokenListMu.Lock()
	tokenList = tokens
	tokenListTime = time.Now()
	tokenListMu.Unlock()

	return tokens, nil
}

// fetchJupiterPrices fetches prices for multiple tokens from Jupiter in one request
func fetchJupiterPrices(tokenAddresses []string) (map[string]float64, error) {
	if len(tokenAddresses) == 0 {
		return nil, nil
	}

	// Batch all token mints (up to 100 per Jupiter limit)
	query := "ids=" + strings.Join(tokenAddresses[:min(len(tokenAddresses), 100)], ",")
	url := fmt.Sprintf("%s?%s", jupiterPriceAPIURL, query)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Jupiter prices: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Jupiter price response: %v", err)
	}
	fmt.Printf("Raw Jupiter Price Response: %s\n", string(body))

	var priceResp JupiterPriceResponse
	if err := json.Unmarshal(body, &priceResp); err != nil {
		return nil, fmt.Errorf("failed to decode Jupiter price response: %v", err)
	}

	prices := make(map[string]float64)
	for addr, data := range priceResp.Data {
		price, err := strconv.ParseFloat(data.Price, 64)
		if err != nil {
			fmt.Printf("Warning: Failed to parse Jupiter price for %s: %v\n", addr, err)
			continue
		}
		prices[addr] = price
		fmt.Printf("Fetched Jupiter price for %s: $%.6f\n", addr, price)
	}
	return prices, nil
}

// fetchPythPrice fetches a price for a token from Pyth as a fallback
func fetchPythPrice(symbol string) (float64, error) {
	priceID, ok := TokenPriceIDs[symbol]
	if !ok {
		return 0, nil // No Pyth ID for this token
	}

	url := fmt.Sprintf("%s?ids[]=%s&parsed=true", pythPriceAPIURL, priceID)
	resp, err := http.Get(url)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch Pyth price for %s: %v", symbol, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read Pyth price response for %s: %v", symbol, err)
	}
	fmt.Printf("Raw Pyth Price Response for %s: %s\n", symbol, string(body))

	var pythResp PythPriceResponse
	if err := json.Unmarshal(body, &pythResp); err != nil {
		return 0, fmt.Errorf("failed to decode Pyth price response for %s: %v", symbol, err)
	}

	for _, item := range pythResp.Parsed {
		if item.ID == priceID {
			price, err := strconv.ParseFloat(item.Price.Price, 64)
			if err != nil {
				return 0, fmt.Errorf("failed to parse Pyth price for %s: %v", symbol, err)
			}
			exponent := float64(item.Price.Expo)
			finalPrice := price * math.Pow10(int(exponent))
			fmt.Printf("Fetched Pyth price for %s: $%.6f\n", symbol, finalPrice)
			return finalPrice, nil
		}
	}
	return 0, nil // Price not found
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// getWalletBalances fetches balances using standard Solana RPC methods
func getWalletBalances(rpcURL, publicKey string) (*WalletResponse, error) {
	// Fetch token list
	tokenList, err := getTokenList()
	if err != nil {
		return nil, fmt.Errorf("failed to get token list: %v", err)
	}

	// Fetch SOL balance with getBalance
	solReq := RPCRequest{
		JSONRPC: "2.0",
		ID:      "sol-balance",
		Method:  "getBalance",
		Params:  []interface{}{publicKey},
	}
	jsonData, _ := json.Marshal(solReq)
	resp, err := http.Post(rpcURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch SOL balance: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Raw getBalance Response: %s\n", string(body))

	var solResp RPCResponseGetBalance
	if err := json.Unmarshal(body, &solResp); err != nil {
		return nil, fmt.Errorf("failed to decode SOL balance: %v", err)
	}
	if solResp.Error != nil {
		return nil, fmt.Errorf("RPC error fetching SOL balance: %s", solResp.Error.Message)
	}

	solBalance := float64(solResp.Result.Value) / 1e9 // Convert lamports to SOL

	// Fetch SPL token accounts with getTokenAccountsByOwner
	tokenReq := RPCRequest{
		JSONRPC: "2.0",
		ID:      "token-accounts",
		Method:  "getTokenAccountsByOwner",
		Params: []interface{}{
			publicKey,
			map[string]string{
				"programId": "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA", // SPL Token Program
			},
			map[string]string{
				"encoding": "jsonParsed",
			},
		},
	}
	jsonData, _ = json.Marshal(tokenReq)
	resp, err = http.Post(rpcURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch token accounts: %v", err)
	}
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)
	fmt.Printf("Raw getTokenAccountsByOwner Response: %s\n", string(body))

	var tokenResp RPCResponseGetTokenAccounts
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token accounts: %v", err)
	}
	if tokenResp.Error != nil {
		return nil, fmt.Errorf("RPC error fetching token accounts: %s", tokenResp.Error.Message)
	}

	// Collect token mints for price fetching
	tokenMints := []string{"So11111111111111111111111111111111111111112"} // SOL
	for _, account := range tokenResp.Result.Value {
		mint := account.Account.Data.Parsed.Info.Mint
		tokenMints = append(tokenMints, mint)
	}

	// Fetch prices from Jupiter in one batch
	jupiterPrices, err := fetchJupiterPrices(tokenMints)
	if err != nil {
		fmt.Printf("Warning: Failed to fetch Jupiter prices: %v\n", err)
		jupiterPrices = make(map[string]float64) // Fallback to empty map
	}

	// Process SOL balance
	solPrice := jupiterPrices["So11111111111111111111111111111111111111112"]
	if solPrice == 0 {
		solPrice, err = fetchPythPrice("SOL")
		if err != nil {
			fmt.Printf("Warning: Failed to fetch Pyth price for SOL: %v\n", err)
		}
	}
	solBalanceUSD := solBalance * solPrice
	fmt.Printf("SOL Balance: %.6f SOL, Price: $%.2f, Value: $%.2f\n", solBalance, solPrice, solBalanceUSD)

	// Process token holdings
	var holdings []Holding
	for _, account := range tokenResp.Result.Value {
		mint := account.Account.Data.Parsed.Info.Mint
		amountStr := account.Account.Data.Parsed.Info.TokenAmount.Amount
		decimals := account.Account.Data.Parsed.Info.TokenAmount.Decimals

		balance, err := strconv.ParseFloat(amountStr, 64)
		if err != nil {
			fmt.Printf("Failed to parse token balance for mint %s: %v\n", mint, err)
			continue
		}
		balance /= math.Pow10(decimals)

		// Skip tiny balances
		if balance < 0.000001 {
			continue
		}

		// Check if token is in Jupiter list and get symbol
		var symbol string
		tokenInList := false
		for _, token := range tokenList {
			if token.Address == mint {
				symbol = token.Symbol
				tokenInList = true
				break
			}
		}
		if !tokenInList {
			continue
		}

		// Fetch USD price with fallback
		usdPrice := jupiterPrices[mint]
		if usdPrice == 0 {
			usdPrice, err = fetchPythPrice(symbol)
			if err != nil {
				fmt.Printf("Warning: Failed to fetch Pyth price for %s: %v\n", symbol, err)
			}
		}
		usdBalance := balance * usdPrice

		holding := Holding{
			Symbol:     symbol,
			Address:    mint,
			Balance:    balance,
			USDPrice:   usdPrice,
			USDBalance: usdBalance,
			Decimals:   decimals, // Added decimals field
		}
		holdings = append(holdings, holding)
		fmt.Printf("Token: %s, Balance: %.6f, USD Price: $%.2f, USD Value: $%.2f\n",
			holding.Symbol, holding.Balance, holding.USDPrice, holding.USDBalance)
	}

	// Sort holdings by USD value (descending)
	sort.Slice(holdings, func(i, j int) bool {
		return holdings[i].USDBalance > holdings[j].USDBalance
	})

	// Prepare response
	response := &WalletResponse{
		SolBalance:    solBalance,
		SolBalanceUSD: solBalanceUSD,
		Assets:        holdings,
	}

	// Update global state
	GetGlobalState().UpdateWalletBalances(response)
	return response, nil
}

// RefreshWalletBalances triggers a balance refresh for the selected wallet
func RefreshWalletBalances() error {
	state := GetGlobalState()
	if state.SelectedWallet == "" {
		return fmt.Errorf("no wallet selected")
	}
	_, err := getWalletBalances(state.RPCURL, state.SelectedWallet)
	return err
}

// NewHomeScreen creates the home screen displaying wallet holdings
func NewHomeScreen() fyne.CanvasObject {
	// Pre-fetch token list in background
	go func() {
		if _, err := getTokenList(); err != nil {
			fmt.Printf("Warning: Failed to fetch token list: %v\n", err)
		}
	}()

	// Get selected wallet
	selectedWallet := GetGlobalState().GetSelectedWallet()
	walletLabel := widget.NewLabel("No wallet selected")
	if selectedWallet != "" {
		walletLabel.SetText(fmt.Sprintf("Loaded Wallet: %s", selectedWallet))
	}

	// UI components
	holdingsLabel := widget.NewLabelWithStyle("Holdings:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	balanceContainer := container.NewVBox()
	scrollContainer := container.NewVScroll(balanceContainer)
	scrollContainer.SetMinSize(fyne.NewSize(300, 400))

	// Define updateBalances function
	updateBalances := func() {
		if selectedWallet == "" {
			balanceContainer.Objects = []fyne.CanvasObject{
				widget.NewLabel("Please select a wallet to view balances."),
			}
			balanceContainer.Refresh()
			return
		}

		balances := GetGlobalState().GetWalletBalances()
		if balances == nil {
			go func() {
				if err := RefreshWalletBalances(); err != nil {
					balanceContainer.Objects = []fyne.CanvasObject{
						widget.NewLabel(fmt.Sprintf("Error fetching balances: %v", err)),
					}
					balanceContainer.Refresh()
					return
				}
				// Fetch updated balances and display them
				balances := GetGlobalState().GetWalletBalances()
				var objects []fyne.CanvasObject
				objects = append(objects,
					widget.NewLabel(fmt.Sprintf("SOL: %.6f ($%.2f)", balances.SolBalance, balances.SolBalanceUSD)),
					widget.NewSeparator(),
				)
				for _, holding := range balances.Assets {
					objects = append(objects,
						widget.NewLabel(fmt.Sprintf("%s: %.6f ($%.2f)", holding.Symbol, holding.Balance, holding.USDBalance)),
						widget.NewSeparator(),
					)
				}
				balanceContainer.Objects = objects
				balanceContainer.Refresh()
			}()
			return
		}

		var objects []fyne.CanvasObject
		objects = append(objects,
			widget.NewLabel(fmt.Sprintf("SOL: %.6f ($%.2f)", balances.SolBalance, balances.SolBalanceUSD)),
			widget.NewSeparator(),
		)
		for _, holding := range balances.Assets {
			objects = append(objects,
				widget.NewLabel(fmt.Sprintf("%s: %.6f ($%.2f)", holding.Symbol, holding.Balance, holding.USDBalance)),
				widget.NewSeparator(),
			)
		}
		balanceContainer.Objects = objects
		balanceContainer.Refresh()
	}

	// Update button
	updateButton := widget.NewButton("Update Balances", func() {
		go func() {
			if err := RefreshWalletBalances(); err != nil {
				balanceContainer.Objects = []fyne.CanvasObject{
					widget.NewLabel(fmt.Sprintf("Error refreshing balances: %v", err)),
				}
				balanceContainer.Refresh()
				return
			}
			updateBalances()
		}()
	})
	updateButton.Importance = widget.HighImportance

	// Assemble UI
	content := container.NewVBox(
		walletLabel,
		holdingsLabel,
		scrollContainer,
		updateButton,
	)

	// Initial balance update if wallet is selected
	if selectedWallet != "" {
		updateBalances()
	}

	return content
}
