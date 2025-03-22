package ui

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unruggable-go/internal/storage"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/mr-tron/base58"
	"github.com/shopspring/decimal"
)

type Asset struct {
	Mint       string
	Decimals   int
	Allocation decimal.Decimal
}

type CalypsoPythPriceResponse struct {
	Parsed []struct {
		ID    string `json:"id"`
		Price struct {
			Price string `json:"price"`
			Expo  int    `json:"expo"`
		} `json:"price"`
	} `json:"parsed"`
}

var ASSETS = map[string]Asset{
	"USDC": {"EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", 6, decimal.NewFromFloat(0.2)},
	"JTO":  {"jtojtomepa8beP8AuQc6eXt5FriJwfFMwQx2v2f9mCL", 9, decimal.NewFromFloat(0.0)},
	"SOL":  {"So11111111111111111111111111111111111111112", 9, decimal.NewFromFloat(0.4)},
	"JUP":  {"JUPyiwrYJFskUPiHa7hkeR8VUtAeFoSYbKedZNsDvCN", 6, decimal.NewFromFloat(0.1)},
	"JLP":  {"27G8MtK7VtTcCHkpASjSDdkWWYfoqT6ggEuKidVJidD4", 6, decimal.NewFromFloat(0.3)},
}

var TOKEN_IDS = map[string]string{
	"SOL": "ef0d8b6fda2ceba41da15d4095d1da392a0d2f8ed0c6c7bc0f4cfac8c280b56d",
	"JUP": "0a0408d619e9380abad35060f9192039ed5042fa6f82301d0e48bb52be830996",
	"JTO": "b43660a5f790c69354b0729a5ef9d50d68f1df92107540210b9cccba1f947cc2",
	"JLP": "c811abc82b4bad1f9bd711a2773ccaa935b03ecef974236942cec5e0eb845a3a",
}

const (
	MAX_RETRIES               = 5
	INITIAL_RETRY_DELAY       = 1 * time.Second
	MAX_RETRY_DELAY           = 30 * time.Second
	RATE_LIMIT_ERROR          = -32429
	KEYPAIR_PATH              = "/Users/hogyzen12/.config/solana/C1PsoU8EPqheU3kv7Gzp6tAoj6UZ5Srtzm4S2f26zss.json"
	CHECK_INTERVAL            = 60
	STASH_ADDRESS             = "StAshdD7TkoNrWqsrbPTwRjCdqaCfMgfVCwKpvaGhuC"
	PYTH_API_ENDPOINT         = "https://hermes.pyth.network/v2/updates/price/latest"
	JUPITER_QUOTE_URL         = "https://quote-api.jup.ag/v6/quote"
	JUPITER_SWAP_INSTRUCTIONS = "https://quote-api.jup.ag/v6/swap-instructions"
	JITO_BUNDLE_URL           = "https://mainnet.block-engine.jito.wtf/api/v1/bundles"
	CLPSO_ENDPOINT            = "https://mainnet.helius-rpc.com/?api-key=001ad922-c61a-4dce-9097-6f8684b0f8c7"
)

var (
	REBALANCE_THRESHOLD    = decimal.NewFromFloat(0.0042)
	STASH_THRESHOLD        = decimal.NewFromFloat(10)
	STASH_AMOUNT           = decimal.NewFromFloat(1)
	DOUBLE_STASH_THRESHOLD = STASH_THRESHOLD.Mul(decimal.NewFromInt(2))
	lastStashValue         *decimal.Decimal
	initialPortfolioValue  *decimal.Decimal
)

type CalypsoBot struct {
	window             fyne.Window
	status             *widget.Label
	log                *widget.Entry
	startStopButton    *widget.Button
	isRunning          bool
	checkInterval      int
	rebalanceThreshold decimal.Decimal
	stashThreshold     decimal.Decimal
	stashAmount        decimal.Decimal
	stashAddress       string
	client             *rpc.Client
	fromAccount        *solana.PrivateKey
	retryDelay         time.Duration
	walletSelect       *widget.Select
	stashSelect        *widget.Select
	app                fyne.App
	container          *fyne.Container
	stashInput         *widget.Entry            // New field for manual stash address input
	allocations        map[string]*widget.Entry // New field to track allocation inputs
	allocationStatus   *widget.Label            // Shows allocation total status
	startButtonEnabled bool                     // Tracks if bot can be started
	validationIcons    map[string]*widget.Label // Visual indicators for each asset
}

func NewCalypsoScreen(window fyne.Window, app fyne.App) fyne.CanvasObject {
	bot := &CalypsoBot{
		window:             window,
		status:             widget.NewLabel("Bot Status: Stopped"),
		log:                widget.NewMultiLineEntry(),
		isRunning:          false,
		checkInterval:      CHECK_INTERVAL,
		rebalanceThreshold: REBALANCE_THRESHOLD,
		stashThreshold:     STASH_THRESHOLD,
		stashAmount:        STASH_AMOUNT,
		client:             rpc.New(CLPSO_ENDPOINT),
		retryDelay:         INITIAL_RETRY_DELAY,
		app:                app,
		allocationStatus:   widget.NewLabel(""),
		startButtonEnabled: false,
		validationIcons:    make(map[string]*widget.Label),
		allocations:        make(map[string]*widget.Entry),
	}

	// Initialize startStopButton early
	bot.startStopButton = widget.NewButton("Start Bot", bot.toggleBot)
	bot.startStopButton.Disable() // Start disabled until validation passes

	bot.log.Disable()
	bot.log.SetMinRowsVisible(9)

	// Get available wallets
	wallets, err := bot.listWalletFiles()
	if err != nil {
		bot.logMessage(fmt.Sprintf("Error listing wallet files: %v", err))
		wallets = []string{}
	}

	// Operating wallet selector
	bot.walletSelect = widget.NewSelect(wallets, func(value string) {
		bot.loadSelectedWallet(value)
	})
	bot.walletSelect.PlaceHolder = "Select Operating Wallet"

	// Stash address input with horizontal radio selection
	stashMethodRadio := widget.NewRadioGroup([]string{"Select from Wallets", "Enter Address"}, func(value string) {
		if value == "Select from Wallets" {
			bot.stashSelect.Show()
			bot.stashInput.Hide()
		} else {
			bot.stashSelect.Hide()
			bot.stashInput.Show()
		}
	})
	stashMethodRadio.Horizontal = true
	bot.startStopButton = widget.NewButton("Start Bot", bot.toggleBot)

	bot.stashSelect = widget.NewSelect(wallets, func(value string) {
		if pubKey, err := bot.getPublicKeyFromWallet(value); err == nil {
			bot.stashAddress = pubKey
			bot.logMessage(fmt.Sprintf("Set stash address to: %s", pubKey))
		}
	})
	bot.stashSelect.PlaceHolder = "Select Stash Wallet"

	bot.stashInput = widget.NewEntry()
	bot.stashInput.SetPlaceHolder("Enter Solana address")
	bot.stashInput.OnChanged = func(value string) {
		if value != "" {
			bot.stashAddress = value
			bot.logMessage(fmt.Sprintf("Set custom stash address to: %s", value))
		}
	}
	bot.stashInput.Hide()

	stashMethodRadio.SetSelected("Select from Wallets")

	allocationsContainer := container.NewGridWithColumns(3) // Changed to 3 columns to include status icons
	allocationsContainer.Add(widget.NewLabelWithStyle("Asset", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	allocationsContainer.Add(widget.NewLabelWithStyle("Allocation (%)", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
	allocationsContainer.Add(widget.NewLabelWithStyle("Status", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))

	for asset := range ASSETS {
		// Asset name
		allocationsContainer.Add(widget.NewLabelWithStyle(asset, fyne.TextAlignLeading, fyne.TextStyle{}))

		// Allocation input
		entry := widget.NewEntry()
		entry.SetText(ASSETS[asset].Allocation.Mul(decimal.NewFromFloat(100)).String())
		entry.OnChanged = func(asset string) func(string) {
			return func(value string) {
				if alloc, err := decimal.NewFromString(value); err == nil {
					details := ASSETS[asset]
					details.Allocation = alloc.Div(decimal.NewFromFloat(100))
					ASSETS[asset] = details
					bot.validateAndUpdateAllocations()
				} else {
					bot.validationIcons[asset].SetText("❌")
					bot.updateStartButtonState(false)
				}
			}
		}(asset)
		bot.allocations[asset] = entry
		allocationsContainer.Add(entry)

		// Status icon
		statusIcon := widget.NewLabel("✓")
		bot.validationIcons[asset] = statusIcon
		allocationsContainer.Add(statusIcon)
	}

	// Add allocation total status below the allocations
	bot.allocationStatus.TextStyle = fyne.TextStyle{Bold: true}
	bot.validateAndUpdateAllocations() // Initial validation

	// Create bot settings section
	settingsContainer := container.NewGridWithColumns(2)

	// Settings labels
	settingsContainer.Add(widget.NewLabelWithStyle("Check Interval (seconds)", fyne.TextAlignLeading, fyne.TextStyle{}))
	checkIntervalEntry := widget.NewEntry()
	checkIntervalEntry.SetText(fmt.Sprintf("%d", bot.checkInterval))
	checkIntervalEntry.OnChanged = func(value string) {
		if interval, _ := strconv.Atoi(value); interval > 0 {
			bot.checkInterval = interval
		}
	}
	settingsContainer.Add(checkIntervalEntry)

	settingsContainer.Add(widget.NewLabelWithStyle("Rebalance Threshold", fyne.TextAlignLeading, fyne.TextStyle{}))
	rebalanceThresholdEntry := widget.NewEntry()
	rebalanceThresholdEntry.SetText(bot.rebalanceThreshold.String())
	rebalanceThresholdEntry.OnChanged = func(value string) {
		if threshold, err := decimal.NewFromString(value); err == nil {
			bot.rebalanceThreshold = threshold
		}
	}
	settingsContainer.Add(rebalanceThresholdEntry)

	settingsContainer.Add(widget.NewLabelWithStyle("Stash Threshold ($)", fyne.TextAlignLeading, fyne.TextStyle{}))
	stashThresholdEntry := widget.NewEntry()
	stashThresholdEntry.SetText(bot.stashThreshold.String())
	stashThresholdEntry.OnChanged = func(value string) {
		if threshold, err := decimal.NewFromString(value); err == nil {
			bot.stashThreshold = threshold
		}
	}
	settingsContainer.Add(stashThresholdEntry)

	settingsContainer.Add(widget.NewLabelWithStyle("Stash Amount ($)", fyne.TextAlignLeading, fyne.TextStyle{}))
	stashAmountEntry := widget.NewEntry()
	stashAmountEntry.SetText(bot.stashAmount.String())
	stashAmountEntry.OnChanged = func(value string) {
		if amount, err := decimal.NewFromString(value); err == nil {
			bot.stashAmount = amount
		}
	}
	settingsContainer.Add(stashAmountEntry)

	allocationsContent := container.NewVBox(
		container.NewPadded(allocationsContainer),
		container.NewPadded(bot.allocationStatus),
	)

	// Create a horizontal container for allocations and settings
	configContainer := container.NewHSplit(
		widget.NewCard("Asset Allocations", "", allocationsContent),
		widget.NewCard("Bot Settings", "", container.NewPadded(settingsContainer)),
	)
	configContainer.SetOffset(0.5) // Set the split to be 50/50

	// Main layout
	bot.container = container.NewVBox(
		widget.NewCard(
			"Wallet Selection",
			"",
			container.NewPadded(bot.walletSelect),
		),
		widget.NewCard(
			"Stash Configuration",
			"",
			container.NewVBox(
				container.NewPadded(stashMethodRadio),
				container.NewPadded(bot.stashSelect),
				container.NewPadded(bot.stashInput),
			),
		),
		configContainer,
		bot.startStopButton,
		bot.status,
		bot.log,
	)

	// Run initial validation after everything is set up
	bot.validateAndUpdateAllocations()

	return bot.container
}

// Updated methods for calypso.go

func (b *CalypsoBot) listWalletFiles() ([]string, error) {
	// Use the storage abstraction to get wallets
	walletStorage := storage.NewWalletStorage(b.app)
	walletMap, err := walletStorage.LoadWallets()
	if err != nil {
		return nil, err
	}

	// Extract wallet IDs from the map
	var walletFiles []string
	for walletID := range walletMap {
		walletFiles = append(walletFiles, walletID)
	}

	// Sort wallets for consistent display
	sort.Strings(walletFiles)

	return walletFiles, nil
}

func (b *CalypsoBot) loadSelectedWallet(walletID string) {
	// Use the storage abstraction to access wallet data
	walletStorage := storage.NewWalletStorage(b.app)
	walletMap, err := walletStorage.LoadWallets()
	if err != nil {
		b.logMessage(fmt.Sprintf("Error loading wallets: %v", err))
		return
	}

	// Get the encrypted wallet data
	encryptedData, ok := walletMap[walletID]
	if !ok {
		b.logMessage(fmt.Sprintf("Wallet %s not found", walletID))
		return
	}

	// Prompt for password
	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Enter wallet password")

	dialog.ShowCustomConfirm("Decrypt Wallet", "Unlock", "Cancel", passwordEntry, func(unlock bool) {
		if !unlock {
			return
		}

		decryptedKey, err := decrypt(encryptedData, passwordEntry.Text)
		if err != nil {
			dialog.ShowError(fmt.Errorf("Failed to decrypt wallet: %v", err), b.window)
			return
		}

		// Convert the decrypted private key to a Solana private key
		privateKey := solana.MustPrivateKeyFromBase58(string(decryptedKey))
		b.fromAccount = &privateKey

		b.logMessage(fmt.Sprintf("Loaded wallet with public key: %s", privateKey.PublicKey().String()))
	}, b.window)
}

func (b *CalypsoBot) getPublicKeyFromWallet(walletID string) (string, error) {
	// This method seems like it's trying to extract the public key from the wallet file
	// In your storage system, the wallet ID itself is the public key, so we can just return it
	return walletID, nil
}

func (b *CalypsoBot) validateAndUpdateAllocations() {
	total := decimal.Zero
	allValid := true

	// Calculate total and validate individual entries
	for asset, entry := range b.allocations {
		value, err := decimal.NewFromString(entry.Text)
		if err != nil || value.IsNegative() {
			b.validationIcons[asset].SetText("❌")
			allValid = false
			continue
		}

		total = total.Add(value)

		// Update icon based on individual value validity
		if value.IsPositive() || value.IsZero() {
			b.validationIcons[asset].SetText("✓")
		} else {
			b.validationIcons[asset].SetText("❌")
			allValid = false
		}
	}

	// Update total status
	if total.Equal(decimal.NewFromFloat(100)) {
		b.allocationStatus.SetText(fmt.Sprintf("Total Allocation: %.1f%% ✓", total.InexactFloat64()))
		if allValid {
			b.updateStartButtonState(true)
			return
		}
	} else {
		b.allocationStatus.SetText(fmt.Sprintf("Total Allocation: %.1f%% ❌ (Must equal 100%%)", total.InexactFloat64()))
	}

	b.updateStartButtonState(false)
}

func (b *CalypsoBot) updateStartButtonState(enabled bool) {
	b.startButtonEnabled = enabled
	if b.startStopButton != nil { // Add nil check
		if enabled {
			b.startStopButton.Enable()
		} else {
			b.startStopButton.Disable()
		}
	}
}

func (b *CalypsoBot) toggleBot() {
	if b.isRunning {
		b.stopBot()
	} else {
		if !b.startButtonEnabled {
			dialog.ShowError(fmt.Errorf("Cannot start bot: Invalid allocations"), b.window)
			return
		}
		b.startBot()
	}
}

func (b *CalypsoBot) startBot() {
	b.isRunning = true
	b.status.SetText("Bot Status: Running")
	b.startStopButton.SetText("Stop Bot")
	b.log.SetText("")
	go b.runBot()
}

func (b *CalypsoBot) stopBot() {
	b.isRunning = false
	b.status.SetText("Bot Status: Stopped")
	b.startStopButton.SetText("Start Bot")
	b.logMessage("Bot stopped.")
}

func (b *CalypsoBot) runBot() {
	b.logMessage("Bot started.")
	for b.isRunning {
		b.performBotCycle()
		time.Sleep(time.Duration(b.checkInterval) * time.Second)
	}
}

func (b *CalypsoBot) performBotCycle() {
	b.logMessage("Starting portfolio check...")

	// Load the keypair
	if b.fromAccount == nil {
		keypair, err := loadKeypair(KEYPAIR_PATH)
		if err != nil {
			b.logMessage(fmt.Sprintf("Failed to load keypair: %v", err))
			return
		}
		b.fromAccount = keypair
	}

	walletAddress := b.fromAccount.PublicKey().String()
	b.logMessage(fmt.Sprintf("Wallet address: %s", walletAddress))

	// Get wallet balances
	balances, err := b.getWalletBalances(walletAddress)
	if err != nil {
		b.logMessage(fmt.Sprintf("Failed to get wallet balances: %v", err))
		return
	}

	// Get prices
	prices, err := b.getPrices()
	if err != nil {
		b.logMessage(fmt.Sprintf("Failed to get prices: %v", err))
		return
	}

	// Calculate portfolio value
	totalValue, usdcValue := b.calculatePortfolioValue(balances, prices)
	b.logMessage(fmt.Sprintf("Total portfolio value: $%s", totalValue.StringFixed(2)))

	// Initialize initialPortfolioValue if it hasn't been set
	if initialPortfolioValue == nil {
		initialPortfolioValue = &totalValue
		b.logMessage(fmt.Sprintf("Initialized initial portfolio value to: $%s", initialPortfolioValue.StringFixed(2)))
	}

	// Calculate DELTA
	delta := decimal.Zero
	if initialPortfolioValue != nil {
		delta = totalValue.Sub(*initialPortfolioValue)
	}
	b.logMessage(fmt.Sprintf("Current DELTA: $%s", delta.StringFixed(2)))

	b.printPortfolio(balances, prices, totalValue)

	rebalanceAmounts := b.calculateRebalanceAmounts(balances, prices, totalValue)
	b.logMessage("Rebalance amounts calculated")

	needRebalance := b.checkNeedRebalance(balances, prices, totalValue)

	// Check for stashing first, independent of rebalancing needs
	if lastStashValue != nil && (delta.GreaterThanOrEqual(STASH_THRESHOLD) || delta.LessThanOrEqual(STASH_THRESHOLD.Neg())) {
		b.logMessage("Stashing threshold reached. Executing stash operation.")
		b.executeStashAndRebalance(rebalanceAmounts, prices, totalValue, usdcValue, delta)
	} else if needRebalance {
		b.logMessage("\nRebalancing needed. Executing rebalance operation.")
		b.executeRebalance(rebalanceAmounts, prices, totalValue, usdcValue)
	} else {
		b.logMessage("\nPortfolio is balanced and no stashing needed.")
	}

	// Update lastStashValue if it hasn't been set yet
	if lastStashValue == nil {
		lastStashValue = &totalValue
		b.logMessage(fmt.Sprintf("Initialized last stash value to: $%s", lastStashValue.StringFixed(2)))
	}
}

func (b *CalypsoBot) getWalletBalances(walletAddress string) (map[string]decimal.Decimal, error) {
	b.logMessage("Fetching wallet balances...")

	requestBody, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "my-id",
		"method":  "getAssetsByOwner",
		"params": map[string]interface{}{
			"ownerAddress": walletAddress,
			"page":         1,
			"limit":        1000,
			"displayOptions": map[string]bool{
				"showFungible":      true,
				"showNativeBalance": true,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	resp, err := http.Post(CLPSO_ENDPOINT, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Read the response body
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// Print the raw response
	b.logMessage("Raw RPC Response:")
	b.logMessage(string(bodyBytes))

	var response CalypsoAssetsResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	// Print the parsed response
	b.logMessage("Parsed RPC Response:")
	b.logMessage(fmt.Sprintf("%+v", response))

	balances := make(map[string]decimal.Decimal)

	for _, item := range response.Result.Items {
		var balance decimal.Decimal
		var balanceStr string
		if err := json.Unmarshal(item.TokenInfo.Balance, &balanceStr); err == nil {
			balance, err = decimal.NewFromString(balanceStr)
			if err != nil {
				b.logMessage(fmt.Sprintf("Failed to parse balance for %s: %v", item.TokenInfo.Symbol, err))
				continue
			}
		} else {
			var balanceNum float64
			if err := json.Unmarshal(item.TokenInfo.Balance, &balanceNum); err == nil {
				balance = decimal.NewFromFloat(balanceNum)
			} else {
				b.logMessage(fmt.Sprintf("Failed to parse balance for %s: %v", item.TokenInfo.Symbol, err))
				continue
			}
		}
		decimals := decimal.New(1, int32(item.TokenInfo.Decimals))
		balances[item.TokenInfo.Symbol] = balance.Div(decimals)

		// Print individual token balance
		b.logMessage(fmt.Sprintf("Token: %s, Raw Balance: %s, Decimals: %d, Parsed Balance: %s",
			item.TokenInfo.Symbol, item.TokenInfo.Balance, item.TokenInfo.Decimals, balances[item.TokenInfo.Symbol]))
	}

	// Add SOL balance
	solBalance := decimal.NewFromInt(response.Result.NativeBalance.Lamports)
	balances["SOL"] = solBalance.Div(decimal.New(1, 9)) // 9 decimals for SOL
	b.logMessage(fmt.Sprintf("SOL Balance: %s lamports, Parsed Balance: %s SOL", solBalance, balances["SOL"]))

	b.logMessage("Wallet balances fetched successfully")
	return balances, nil
}

func (b *CalypsoBot) getPrices() (map[string]decimal.Decimal, error) {
	b.logMessage("Fetching asset prices...")

	// Construct the URL with parameters
	baseURL, err := url.Parse(PYTH_API_ENDPOINT)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %v", err)
	}

	params := url.Values{}
	for _, id := range TOKEN_IDS {
		params.Add("ids[]", id)
	}
	params.Set("parsed", "true")
	baseURL.RawQuery = params.Encode()

	// Make the HTTP request
	resp, err := http.Get(baseURL.String())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch prices: %v", err)
	}
	defer resp.Body.Close()

	// Parse the response
	var pythResp CalypsoPythPriceResponse
	if err := json.NewDecoder(resp.Body).Decode(&pythResp); err != nil {
		return nil, fmt.Errorf("failed to decode price response: %v", err)
	}

	// Calculate prices
	prices := make(map[string]decimal.Decimal)
	for _, item := range pythResp.Parsed {
		for token, id := range TOKEN_IDS {
			if id == item.ID {
				price, err := decimal.NewFromString(item.Price.Price)
				if err != nil {
					return nil, fmt.Errorf("failed to parse price for %s: %v", token, err)
				}
				exponent := decimal.New(1, int32(item.Price.Expo))
				prices[token] = price.Mul(exponent)
				break
			}
		}
	}

	// Add USDC price
	prices["USDC"] = decimal.NewFromFloat(1.0)

	b.logMessage("Asset prices fetched successfully")
	b.logMessage(fmt.Sprintf("Fetched prices: %+v", prices))

	return prices, nil
}

func (b *CalypsoBot) calculatePortfolioValue(balances, prices map[string]decimal.Decimal) (decimal.Decimal, decimal.Decimal) {
	totalValue := decimal.Zero
	for asset, balance := range balances {
		totalValue = totalValue.Add(balance.Mul(prices[asset]))
	}

	usdcValue := balances["USDC"].Mul(prices["USDC"])
	return totalValue, usdcValue
}

func (b *CalypsoBot) calculateRebalanceAmounts(balances, prices map[string]decimal.Decimal, totalValue decimal.Decimal) map[string]decimal.Decimal {
	rebalanceAmounts := make(map[string]decimal.Decimal)

	for asset, details := range ASSETS {
		//currentValue := balances[asset].Mul(prices[asset])
		targetValue := totalValue.Mul(details.Allocation)
		targetAmount := targetValue.Div(prices[asset])
		rebalanceAmount := targetAmount.Sub(balances[asset])
		rebalanceAmounts[asset] = rebalanceAmount.Round(6)
	}

	return rebalanceAmounts
}

func (b *CalypsoBot) checkNeedRebalance(balances, prices map[string]decimal.Decimal, totalValue decimal.Decimal) bool {
	for asset, details := range ASSETS {
		currentAllocation := balances[asset].Mul(prices[asset]).Div(totalValue)
		if currentAllocation.Sub(details.Allocation).Abs().GreaterThan(b.rebalanceThreshold) {
			return true
		}
	}
	return false
}

func (b *CalypsoBot) calculateTrades(rebalanceAmounts, prices map[string]decimal.Decimal) []Trade {
	var trades []Trade
	for asset, amount := range rebalanceAmounts {
		if asset != "USDC" && amount.Abs().GreaterThan(decimal.NewFromFloat(0.01)) {
			tradeValue := amount.Abs().Mul(prices[asset])
			if amount.GreaterThan(decimal.Zero) {
				trades = append(trades, Trade{
					From:       "USDC",
					To:         asset,
					Amount:     tradeValue,
					FromAmount: tradeValue,
					ToAmount:   amount,
				})
			} else {
				trades = append(trades, Trade{
					From:       asset,
					To:         "USDC",
					Amount:     tradeValue,
					FromAmount: amount.Abs(),
					ToAmount:   tradeValue,
				})
			}
		}
	}
	return trades
}

func (b *CalypsoBot) printTrades(trades []Trade) {
	b.logMessage("\nExecuting the following trades:")
	b.logMessage(strings.Repeat("-", 70))
	b.logMessage("From   To     From Amount      To Amount      Value ($)")
	b.logMessage(strings.Repeat("-", 70))
	for _, trade := range trades {
		b.logMessage(fmt.Sprintf("%-6s %-6s %15s %15s %12s",
			trade.From, trade.To,
			trade.FromAmount.StringFixed(6),
			trade.ToAmount.StringFixed(6),
			trade.Amount.StringFixed(2)))
	}
	b.logMessage(strings.Repeat("-", 70))
}

func (b *CalypsoBot) verifyTransactions(doubleStashTriggered bool) {
	b.logMessage("Waiting for 15 seconds before verifying the transactions...")
	time.Sleep(15 * time.Second)

	balances, err := b.getWalletBalances(b.fromAccount.PublicKey().String())
	if err != nil {
		b.logMessage(fmt.Sprintf("Failed to get updated balances: %v", err))
		return
	}

	prices, err := b.getPrices()
	if err != nil {
		b.logMessage(fmt.Sprintf("Failed to get updated prices: %v", err))
		return
	}

	updatedTotalValue, _ := b.calculatePortfolioValue(balances, prices)

	b.logMessage("\nUpdated portfolio after operation:")
	b.printPortfolio(balances, prices, updatedTotalValue)

	rebalanceSuccessful := b.checkNeedRebalance(balances, prices, updatedTotalValue)

	if !rebalanceSuccessful {
		b.logMessage("Operation was successful.")
		lastStashValue = &updatedTotalValue
		initialPortfolioValue = &updatedTotalValue
		b.logMessage(fmt.Sprintf("Updated last stash value to: $%s", lastStashValue.StringFixed(2)))
		b.logMessage(fmt.Sprintf("Reset initial portfolio value to: $%s", initialPortfolioValue.StringFixed(2)))

		if doubleStashTriggered {
			b.logMessage("Double stash completed.")
		}
	} else {
		b.logMessage("Operation may not have been fully successful. Please check the updated portfolio.")
	}
}

func (b *CalypsoBot) printPortfolio(balances, prices map[string]decimal.Decimal, totalValue decimal.Decimal) {
	b.logMessage("\nCurrent Portfolio:")
	b.logMessage("------------------")
	b.logMessage("Asset  Balance      Value ($)   Allocation  Target")
	b.logMessage(strings.Repeat("-", 57))
	for asset, details := range ASSETS {
		balance := balances[asset]
		value := balance.Mul(prices[asset])
		allocation := value.Div(totalValue).Mul(decimal.NewFromInt(100))
		targetAllocation := details.Allocation.Mul(decimal.NewFromInt(100))
		b.logMessage(fmt.Sprintf("%-6s %12s %12s %11s%% %8s%%",
			asset,
			balance.StringFixed(3),
			value.StringFixed(2),
			allocation.StringFixed(2),
			targetAllocation.StringFixed(2)))
	}
	b.logMessage(strings.Repeat("-", 57))
	b.logMessage(fmt.Sprintf("%-6s %12s %12s %11s %8s",
		"Total", "", totalValue.StringFixed(2), "100.00%", "100.00%"))
}

func (b *CalypsoBot) logMessage(message string) {
	log.Println(message)
	b.log.SetText(b.log.Text + message + "\n")
}

func loadKeypair(path string) (*solana.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read keypair file: %v", err)
	}

	var secretKey []byte
	err = json.Unmarshal(data, &secretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal keypair: %v", err)
	}

	privateKey := solana.PrivateKey(secretKey)
	return &privateKey, nil
}

func decrypt(encryptedData string, password string) ([]byte, error) {
	key := []byte(padKey(password))
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	ciphertext, err := hex.DecodeString(encryptedData)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

func (b *CalypsoBot) getJupiterSwapInstructions(fromAccountPublicKey solana.PublicKey, inputMint, outputMint string, amountLamports int64, slippageBps int) (map[string]interface{}, error) {
	b.logMessage(fmt.Sprintf("Getting Jupiter swap instructions for %s to %s...", inputMint, outputMint))

	quoteURL := fmt.Sprintf("%s?onlyDirectRoutes=true&inputMint=%s&outputMint=%s&amount=%d&slippageBps=%d",
		JUPITER_QUOTE_URL, inputMint, outputMint, amountLamports, slippageBps)

	quoteResp, err := http.Get(quoteURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get Jupiter quote: %v", err)
	}
	defer quoteResp.Body.Close()

	var quoteData map[string]interface{}
	if err := json.NewDecoder(quoteResp.Body).Decode(&quoteData); err != nil {
		return nil, fmt.Errorf("failed to decode Jupiter quote: %v", err)
	}

	swapBody := map[string]interface{}{
		"userPublicKey":             fromAccountPublicKey.String(),
		"quoteResponse":             quoteData,
		"wrapAndUnwrapSol":          true,
		"prioritizationFeeLamports": 0,
		"dynamicComputeUnitLimit":   true,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(swapBody); err != nil {
		return nil, fmt.Errorf("failed to encode swap body: %v", err)
	}

	swapResp, err := http.Post(JUPITER_SWAP_INSTRUCTIONS, "application/json", &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to get swap instructions: %v", err)
	}
	defer swapResp.Body.Close()

	var swapData map[string]interface{}
	if err := json.NewDecoder(swapResp.Body).Decode(&swapData); err != nil {
		return nil, fmt.Errorf("failed to decode swap instructions: %v", err)
	}

	b.logMessage("Jupiter swap instructions fetched successfully")
	return swapData, nil
}

func (b *CalypsoBot) createSwapTransaction(fromAccount *solana.PrivateKey, inputAsset, outputAsset string, amount decimal.Decimal) (*solana.Transaction, error) {
	b.logMessage(fmt.Sprintf("Creating swap transaction for %s to %s...", inputAsset, outputAsset))

	inputMint := ASSETS[inputAsset].Mint
	outputMint := ASSETS[outputAsset].Mint
	amountLamports := amount.Mul(decimal.New(1, int32(ASSETS[inputAsset].Decimals))).IntPart()

	b.logMessage(fmt.Sprintf("Input Mint: %s, Output Mint: %s, Amount (lamports): %d", inputMint, outputMint, amountLamports))

	swapInstructions, err := b.getJupiterSwapInstructions(fromAccount.PublicKey(), inputMint, outputMint, amountLamports, 100)
	if err != nil {
		b.logMessage(fmt.Sprintf("Error getting Jupiter swap instructions: %v", err))
		return nil, err
	}
	b.logMessage("Jupiter swap instructions fetched successfully")
	b.logMessage(fmt.Sprintf("Swap Instructions: %+v", swapInstructions))

	recentBlockhash, err := b.client.GetRecentBlockhash(context.Background(), rpc.CommitmentFinalized)
	if err != nil {
		b.logMessage(fmt.Sprintf("Failed to get recent blockhash: %v", err))
		return nil, fmt.Errorf("failed to get recent blockhash: %v", err)
	}
	b.logMessage(fmt.Sprintf("Recent blockhash: %s", recentBlockhash.Value.Blockhash))

	builder := solana.NewTransactionBuilder()
	builder.SetFeePayer(fromAccount.PublicKey())
	builder.SetRecentBlockHash(recentBlockhash.Value.Blockhash)

	// Add setup instructions
	setupInstructions, ok := swapInstructions["setupInstructions"].([]interface{})
	if !ok {
		b.logMessage("Error: setupInstructions is not of expected type")
		return nil, fmt.Errorf("setupInstructions is not of expected type")
	}
	for i, instruction := range setupInstructions {
		instData, ok := instruction.(map[string]interface{})
		if !ok {
			b.logMessage(fmt.Sprintf("Error: setup instruction %d is not of expected type", i))
			return nil, fmt.Errorf("setup instruction %d is not of expected type", i)
		}
		builder.AddInstruction(b.createTransactionInstruction(instData))
	}
	b.logMessage(fmt.Sprintf("Added %d setup instructions", len(setupInstructions)))

	// Add swap instruction
	swapInstruction, ok := swapInstructions["swapInstruction"].(map[string]interface{})
	if !ok {
		b.logMessage("Error: swapInstruction is not of expected type")
		return nil, fmt.Errorf("swapInstruction is not of expected type")
	}
	builder.AddInstruction(b.createTransactionInstruction(swapInstruction))
	b.logMessage("Added swap instruction")

	// Add cleanup instruction if present
	if cleanupInst, ok := swapInstructions["cleanupInstruction"]; ok && cleanupInst != nil {
		cleanupInstData, ok := cleanupInst.(map[string]interface{})
		if !ok {
			b.logMessage("Error: cleanupInstruction is not of expected type")
			return nil, fmt.Errorf("cleanupInstruction is not of expected type")
		}
		builder.AddInstruction(b.createTransactionInstruction(cleanupInstData))
		b.logMessage("Added cleanup instruction")
	} else {
		b.logMessage("No cleanup instruction present or it's nil")
	}

	// Build the transaction
	tx, err := builder.Build()
	if err != nil {
		b.logMessage(fmt.Sprintf("Failed to build transaction: %v", err))
		return nil, fmt.Errorf("failed to build transaction: %v", err)
	}
	b.logMessage("Transaction built successfully")

	// Sign the transaction
	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(fromAccount.PublicKey()) {
			return fromAccount
		}
		return nil
	})
	if err != nil {
		b.logMessage(fmt.Sprintf("Failed to sign transaction: %v", err))
		return nil, fmt.Errorf("failed to sign transaction: %v", err)
	}

	b.logMessage("Swap transaction created and signed successfully")
	return tx, nil
}

func (b *CalypsoBot) createTransactionInstruction(instructionData map[string]interface{}) solana.Instruction {
	programID := solana.MustPublicKeyFromBase58(instructionData["programId"].(string))
	accounts := solana.AccountMetaSlice{}
	for _, acc := range instructionData["accounts"].([]interface{}) {
		accData := acc.(map[string]interface{})
		pubkey := solana.MustPublicKeyFromBase58(accData["pubkey"].(string))
		accounts = append(accounts, &solana.AccountMeta{
			PublicKey:  pubkey,
			IsSigner:   accData["isSigner"].(bool),
			IsWritable: accData["isWritable"].(bool),
		})
	}
	data, _ := base64.StdEncoding.DecodeString(instructionData["data"].(string))

	return solana.NewInstruction(programID, accounts, data)
}

func (b *CalypsoBot) sendBundle(transactions []*solana.Transaction) (string, error) {
	b.logMessage("Preparing transaction bundle...")

	encodedTransactions := make([]string, len(transactions))
	for i, tx := range transactions {
		encodedTx, err := tx.MarshalBinary()
		if err != nil {
			return "", fmt.Errorf("failed to encode transaction: %v", err)
		}
		encodedTransactions[i] = base58.Encode(encodedTx)
		b.logMessage(fmt.Sprintf("Encoded transaction %d: %s", i+1, encodedTransactions[i]))
	}

	bundleData := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "sendBundle",
		"params":  []interface{}{encodedTransactions},
	}

	bundleJSON, err := json.MarshalIndent(bundleData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal bundle data: %v", err)
	}

	b.logMessage("Sending the following bundle to Jito:")
	b.logMessage(string(bundleJSON))

	resp, err := http.Post(JITO_BUNDLE_URL, "application/json", bytes.NewBuffer(bundleJSON))
	if err != nil {
		return "", fmt.Errorf("failed to send bundle: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	b.logMessage("Received response from Jito:")
	b.logMessage(string(respBody))

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to decode bundle response: %v", err)
	}

	if errorData, ok := result["error"]; ok {
		return "", fmt.Errorf("bundle error: %v", errorData)
	}

	bundleID, ok := result["result"].(string)
	if !ok {
		return "", fmt.Errorf("invalid bundle response")
	}

	b.logMessage("Transaction bundle sent successfully")
	return bundleID, nil
}

func (b *CalypsoBot) executeStashAndRebalance(rebalanceAmounts, prices map[string]decimal.Decimal, totalValue, usdcValue, delta decimal.Decimal) {
	b.logMessage("Executing stash and rebalance operation...")
	stashAmount := b.stashAmount
	doubleStashTriggered := false

	if delta.GreaterThanOrEqual(DOUBLE_STASH_THRESHOLD) {
		doubleStashTriggered = true
		stashAmount = b.stashAmount.Mul(decimal.NewFromInt(2))
		b.logMessage("Double stash threshold reached.")
	}

	b.logMessage(fmt.Sprintf("Stashing $%s USDC to %s", stashAmount.String(), b.stashAddress))

	trades := b.calculateTrades(rebalanceAmounts, prices)
	if len(trades) > 0 {
		b.printTrades(trades)
	} else {
		b.logMessage("No trades needed for rebalancing after stash.")
	}

	// Create stash transaction
	stashTx, err := b.createTipTransaction()
	if err != nil {
		b.logMessage(fmt.Sprintf("Failed to create stash transaction: %v", err))
		return
	}

	// Create rebalance transactions
	swapTransactions, err := b.createRebalanceTransactions(rebalanceAmounts, prices)
	if err != nil {
		b.logMessage(fmt.Sprintf("Failed to create rebalance transactions: %v", err))
		return
	}

	// Combine all transactions
	allTransactions := append(swapTransactions, stashTx)

	// Send the bundle
	bundleID, err := b.sendBundle(allTransactions)
	if err != nil {
		b.logMessage(fmt.Sprintf("Failed to send transaction bundle: %v", err))
		return
	}

	b.logMessage(fmt.Sprintf("Bundle submitted with ID: %s", bundleID))
	b.logMessage(fmt.Sprintf("Stashed $%s to %s", stashAmount.String(), b.stashAddress))
	b.logMessage(fmt.Sprintf("Processed %d swap(s) and 1 stash transaction.", len(swapTransactions)))

	// Verify the transactions
	b.verifyTransactions(doubleStashTriggered)
}

func (b *CalypsoBot) executeRebalance(rebalanceAmounts, prices map[string]decimal.Decimal, totalValue, usdcValue decimal.Decimal) {
	b.logMessage("Executing rebalance operation...")

	trades := b.calculateTrades(rebalanceAmounts, prices)
	if len(trades) > 0 {
		b.printTrades(trades)
	} else {
		b.logMessage("No trades needed for rebalancing.")
		return
	}

	swapTransactions, err := b.createRebalanceTransactions(rebalanceAmounts, prices)
	if err != nil {
		b.logMessage(fmt.Sprintf("Failed to create rebalance transactions: %v", err))
		return
	}

	// Create tip transaction
	tipTx, err := b.createTipTransaction()
	if err != nil {
		b.logMessage(fmt.Sprintf("Failed to create tip transaction: %v", err))
		return
	}

	// Combine swap transactions with tip transaction
	allTransactions := append(swapTransactions, tipTx)

	// Send the bundle
	bundleID, err := b.sendBundle(allTransactions)
	if err != nil {
		b.logMessage(fmt.Sprintf("Failed to send transaction bundle: %v", err))
		return
	}

	b.logMessage(fmt.Sprintf("Bundle submitted with ID: %s", bundleID))
	b.logMessage(fmt.Sprintf("Processed %d swap(s) and 1 tip transaction.", len(swapTransactions)))

	// Verify the transactions
	b.verifyTransactions(false)
}

func (b *CalypsoBot) createRebalanceTransactions(rebalanceAmounts, prices map[string]decimal.Decimal) ([]*solana.Transaction, error) {
	var swapTransactions []*solana.Transaction

	for asset, amount := range rebalanceAmounts {
		if asset != "USDC" && amount.Abs().GreaterThan(decimal.NewFromFloat(0.01)) {
			var tx *solana.Transaction
			var err error

			if amount.GreaterThan(decimal.Zero) {
				usdcAmount := amount.Mul(prices[asset]).Round(6)
				tx, err = b.createSwapTransaction(b.fromAccount, "USDC", asset, usdcAmount)
			} else {
				tx, err = b.createSwapTransaction(b.fromAccount, asset, "USDC", amount.Abs())
			}

			if err != nil {
				return nil, fmt.Errorf("failed to create swap transaction for %s: %v", asset, err)
			}

			swapTransactions = append(swapTransactions, tx)
		}
	}

	return swapTransactions, nil
}

func (b *CalypsoBot) createTipTransaction() (*solana.Transaction, error) {
	b.logMessage("Creating tip transaction...")

	recentBlockhash, err := b.client.GetRecentBlockhash(context.Background(), rpc.CommitmentFinalized)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent blockhash: %v", err)
	}

	builder := solana.NewTransactionBuilder()
	builder.SetFeePayer(b.fromAccount.PublicKey())
	builder.SetRecentBlockHash(recentBlockhash.Value.Blockhash)

	// Add tip transfers
	tipRecipients := []string{
		"juLesoSmdTcRtzjCzYzRoHrnF8GhVu6KCV7uxq7nJGp",
		"DttWaMuVvTiduZRnguLF7jNxTgiMBZ1hyAumKUiL2KRL",
	}

	for _, recipient := range tipRecipients {
		tipInstruction := system.NewTransferInstruction(
			100_000, // 0.0001 SOL
			b.fromAccount.PublicKey(),
			solana.MustPublicKeyFromBase58(recipient),
		).Build()

		builder.AddInstruction(tipInstruction)
	}

	// Build the transaction
	tx, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build transaction: %v", err)
	}

	// Sign the transaction
	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(b.fromAccount.PublicKey()) {
			return b.fromAccount
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to sign tip transaction: %v", err)
	}

	b.logMessage("Tip transaction created successfully")
	return tx, nil
}

type Trade struct {
	From       string
	To         string
	Amount     decimal.Decimal
	FromAmount decimal.Decimal
	ToAmount   decimal.Decimal
}
