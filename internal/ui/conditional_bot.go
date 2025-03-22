package ui

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
	"unruggable-go/internal/storage"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/mr-tron/base58"
	"github.com/shopspring/decimal"
)

const (
	CNDTNL_JUPITER_PRICE_URL  = "https://api.jup.ag/price/v2"       // Updated URL
	CNDTNL_JUPITER_QUOTE_URL  = "https://quote-api.jup.ag/v6/quote" // Updated quote API URL
	CNDTNL_JUPITER_SWAP_INSTR = "https://quote-api.jup.ag/v6/swap-instructions"
	CNDTNL_JITO_BUNDLE_URL    = "https://mainnet.block-engine.jito.wtf/api/v1/bundles"
	CNDTNL_ENDPOINT           = "https://special-blue-fog.solana-mainnet.quiknode.pro/d009d548b4b9dd9f062a8124a868fb915937976c/"
	CNDTNL_CHECK_INTERVAL     = 60
)

// Supported price condition operators
var ConditionOperators = []string{"Greater Than (>)", "Less Than (<)", "Equal To (=)", "Greater Than or Equal (>=)", "Less Than or Equal (<=)"}

// Supported action types
var ActionTypes = []string{"Buy", "Sell", "Send"}

// Supported asset types for monitoring
var MonitoredAssets = map[string]PriceAsset{
	"SOL":  {"SOL", "So11111111111111111111111111111111111111112", 9},
	"JUP":  {"JUP", "JUPyiwrYJFskUPiHa7hkeR8VUtAeFoSYbKedZNsDvCN", 6},
	"JTO":  {"JTO", "jtojtomepa8beP8AuQc6eXt5FriJwfFMwQx2v2f9mCL", 9},
	"USDC": {"USDC", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", 6},
}

// Supported trading pairs
var TradingPairs = map[string]TradingPair{
	"SOL/USDC": {"SOL", "USDC", "So11111111111111111111111111111111111111112", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", 9, 6},
	"JUP/USDC": {"JUP", "USDC", "JUPyiwrYJFskUPiHa7hkeR8VUtAeFoSYbKedZNsDvCN", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", 6, 6},
	"JTO/USDC": {"JTO", "USDC", "jtojtomepa8beP8AuQc6eXt5FriJwfFMwQx2v2f9mCL", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", 9, 6},
	"USDC/SOL": {"USDC", "SOL", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", "So11111111111111111111111111111111111111112", 6, 9},
	"USDC/JUP": {"USDC", "JUP", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", "JUPyiwrYJFskUPiHa7hkeR8VUtAeFoSYbKedZNsDvCN", 6, 6},
	"USDC/JTO": {"USDC", "JTO", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", "jtojtomepa8beP8AuQc6eXt5FriJwfFMwQx2v2f9mCL", 6, 9},
}

// Types for the conditional bot implementation
type PriceAsset struct {
	Symbol    string
	TokenMint string // Now stores the token mint address
	Decimals  int
}

type TradingPair struct {
	BaseSymbol    string
	QuoteSymbol   string
	BaseMint      string
	QuoteMint     string
	BaseDecimals  int
	QuoteDecimals int
}

type PriceCondition struct {
	Asset     string
	Operator  string
	Price     decimal.Decimal
	Triggered bool
}

type TradeAction struct {
	Type     string
	Pair     string
	Amount   decimal.Decimal
	Executed bool
}

type ConditionalTrade struct {
	ID         string
	Condition  PriceCondition
	Action     TradeAction
	Active     bool
	CreatedAt  time.Time
	ExecutedAt *time.Time
}

type ConditionalBotScreen struct {
	window          fyne.Window
	app             fyne.App
	log             *widget.Entry
	status          *widget.Label
	startStopButton *widget.Button
	isRunning       bool
	trades          []*ConditionalTrade
	activeTrades    sync.Map
	tradesContainer *fyne.Container
	client          *rpc.Client
	container       *fyne.Container
	walletSelect    *widget.Select
	fromAccount     *solana.PrivateKey

	// Form fields for creating new conditions
	assetSelect       *widget.Select
	operatorSelect    *widget.Select
	priceEntry        *widget.Entry
	actionTypeSelect  *widget.Select
	tradingPairSelect *widget.Select
	amountEntry       *widget.Entry
}

// Create a new conditional bot screen
func NewConditionalBotScreen(window fyne.Window, app fyne.App) fyne.CanvasObject {
	bot := &ConditionalBotScreen{
		window:    window,
		app:       app,
		log:       widget.NewMultiLineEntry(),
		status:    widget.NewLabel("Bot Status: Stopped"),
		isRunning: false,
		trades:    make([]*ConditionalTrade, 0),
		client:    rpc.New(CNDTNL_ENDPOINT),
	}

	bot.log.Disable()
	bot.log.SetMinRowsVisible(9)

	// Initialize startStopButton
	bot.startStopButton = widget.NewButton("Start Bot", bot.toggleBot)
	bot.startStopButton.Disable() // Start disabled until wallet is selected

	// Get available wallets
	wallets, err := bot.listWalletFiles()
	if err != nil {
		bot.logMessage(fmt.Sprintf("Error listing wallet files: %v", err))
		wallets = []string{}
	}

	// Wallet selector
	bot.walletSelect = widget.NewSelect(wallets, func(value string) {
		bot.logMessage(fmt.Sprintf("Selected wallet: %s", value))
		bot.loadSelectedWallet(value)
	})
	bot.walletSelect.PlaceHolder = "Select Operating Wallet"

	// Initialize form fields for creating conditions
	bot.assetSelect = widget.NewSelect(getKeys(MonitoredAssets), func(value string) {})
	bot.assetSelect.PlaceHolder = "Select Asset to Monitor"

	bot.operatorSelect = widget.NewSelect(ConditionOperators, func(value string) {})
	bot.operatorSelect.PlaceHolder = "Select Condition"

	bot.priceEntry = widget.NewEntry()
	bot.priceEntry.SetPlaceHolder("Enter Price (e.g., 100.50)")

	bot.actionTypeSelect = widget.NewSelect(ActionTypes, func(value string) {
		// Update available trading pairs based on action type
		bot.updateTradingPairs(value)
	})
	bot.actionTypeSelect.PlaceHolder = "Select Action"

	bot.tradingPairSelect = widget.NewSelect(getKeys(TradingPairs), func(value string) {})
	bot.tradingPairSelect.PlaceHolder = "Select Trading Pair"

	bot.amountEntry = widget.NewEntry()
	bot.amountEntry.SetPlaceHolder("Enter Amount (e.g., 100 USDC or 1 SOL)")

	// Button to add a new condition
	addButton := widget.NewButton("Add Condition", bot.addNewCondition)

	// Trades container for displaying active conditions
	bot.tradesContainer = container.NewVBox()

	// Form for adding new conditions
	formCard := widget.NewCard("Create New Condition", "",
		container.NewVBox(
			container.NewGridWithColumns(2,
				widget.NewLabel("Asset to Monitor:"),
				bot.assetSelect,
			),
			container.NewGridWithColumns(2,
				widget.NewLabel("Condition:"),
				bot.operatorSelect,
			),
			container.NewGridWithColumns(2,
				widget.NewLabel("Target Price:"),
				bot.priceEntry,
			),
			container.NewGridWithColumns(2,
				widget.NewLabel("Action:"),
				bot.actionTypeSelect,
			),
			container.NewGridWithColumns(2,
				widget.NewLabel("Trading Pair:"),
				bot.tradingPairSelect,
			),
			container.NewGridWithColumns(2,
				widget.NewLabel("Amount:"),
				bot.amountEntry,
			),
			addButton,
		),
	)

	// Active conditions display
	activeConditionsCard := widget.NewCard("Active Conditions", "",
		container.NewVScroll(bot.tradesContainer),
	)

	// Main layout
	bot.container = container.NewVBox(
		widget.NewCard("Wallet Selection", "", container.NewPadded(bot.walletSelect)),
		formCard,
		activeConditionsCard,
		bot.startStopButton,
		bot.status,
		bot.log,
	)

	// Load any existing conditions (in a real app these would be persisted)
	bot.refreshTradesDisplay()

	return bot.container
}

// Get map keys as a string slice
func getKeys(m interface{}) []string {
	var keys []string

	switch v := m.(type) {
	case map[string]PriceAsset:
		for k := range v {
			keys = append(keys, k)
		}
	case map[string]TradingPair:
		for k := range v {
			keys = append(keys, k)
		}
	}

	return keys
}

// Update available trading pairs based on action type
func (b *ConditionalBotScreen) updateTradingPairs(actionType string) {
	// The implementation can be enhanced to filter pairs based on the action type
	// For simplicity, we'll just use all pairs for now
	b.tradingPairSelect.Options = getKeys(TradingPairs)
	b.tradingPairSelect.Refresh()
}

func (b *ConditionalBotScreen) listWalletFiles() ([]string, error) {
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

func (b *ConditionalBotScreen) loadSelectedWallet(walletID string) {
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

		// Enable the start button once a wallet is loaded
		b.startStopButton.Enable()
	}, b.window)
}

// Add a new conditional trade
func (b *ConditionalBotScreen) addNewCondition() {
	// Validate form fields
	if b.assetSelect.Selected == "" ||
		b.operatorSelect.Selected == "" ||
		b.priceEntry.Text == "" ||
		b.actionTypeSelect.Selected == "" ||
		b.tradingPairSelect.Selected == "" ||
		b.amountEntry.Text == "" {
		dialog.ShowError(fmt.Errorf("Please fill in all fields"), b.window)
		return
	}

	// Parse price
	price, err := decimal.NewFromString(b.priceEntry.Text)
	if err != nil {
		dialog.ShowError(fmt.Errorf("Invalid price value: %v", err), b.window)
		return
	}

	// Parse amount
	amount, err := decimal.NewFromString(b.amountEntry.Text)
	if err != nil {
		dialog.ShowError(fmt.Errorf("Invalid amount value: %v", err), b.window)
		return
	}

	// Create new trade condition
	trade := &ConditionalTrade{
		ID: fmt.Sprintf("trade_%d", time.Now().UnixNano()),
		Condition: PriceCondition{
			Asset:     b.assetSelect.Selected,
			Operator:  b.operatorSelect.Selected,
			Price:     price,
			Triggered: false,
		},
		Action: TradeAction{
			Type:     b.actionTypeSelect.Selected,
			Pair:     b.tradingPairSelect.Selected,
			Amount:   amount,
			Executed: false,
		},
		Active:    true,
		CreatedAt: time.Now(),
	}

	// Add to trades list
	b.trades = append(b.trades, trade)
	b.activeTrades.Store(trade.ID, trade)

	b.logMessage(fmt.Sprintf("Added new condition: %s $%s %s -> %s %s $%s",
		trade.Condition.Asset,
		trade.Condition.Price.String(),
		simplifyOperator(trade.Condition.Operator),
		trade.Action.Type,
		trade.Action.Amount.String(),
		trade.Action.Pair))

	// Refresh the display
	b.refreshTradesDisplay()

	// Clear form fields
	b.clearForm()
}

// Convert operator display text to symbol
func simplifyOperator(operator string) string {
	switch operator {
	case "Greater Than (>)":
		return ">"
	case "Less Than (<)":
		return "<"
	case "Equal To (=)":
		return "="
	case "Greater Than or Equal (>=)":
		return ">="
	case "Less Than or Equal (<=)":
		return "<="
	default:
		return operator
	}
}

// Clear form fields after adding a condition
func (b *ConditionalBotScreen) clearForm() {
	b.assetSelect.ClearSelected()
	b.operatorSelect.ClearSelected()
	b.priceEntry.SetText("")
	b.actionTypeSelect.ClearSelected()
	b.tradingPairSelect.ClearSelected()
	b.amountEntry.SetText("")
}

func (b *ConditionalBotScreen) createTradeCard(trade *ConditionalTrade) fyne.CanvasObject {
	// Create a more visually distinct card with better spacing and styling

	// Add an icon to indicate status
	var statusIcon *widget.Icon
	var statusText string

	if !trade.Active {
		statusIcon = widget.NewIcon(theme.CancelIcon())
		statusText = "Inactive"
	} else if trade.Action.Executed {
		statusIcon = widget.NewIcon(theme.ConfirmIcon())
		statusText = "Executed"
	} else if trade.Condition.Triggered {
		statusIcon = widget.NewIcon(theme.WarningIcon())
		statusText = "Triggered"
	} else {
		statusIcon = widget.NewIcon(theme.RadioButtonIcon())
		statusText = "Monitoring"
	}

	// Status row with icon and label
	statusLabel := widget.NewLabelWithStyle(statusText, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	statusContainer := container.NewHBox(
		statusIcon,
		statusLabel,
	)

	// Main condition text - make it more readable
	conditionText := fmt.Sprintf("When %s price %s $%s",
		trade.Condition.Asset,
		simplifyOperator(trade.Condition.Operator),
		trade.Condition.Price.String())

	conditionLabel := widget.NewLabelWithStyle(conditionText, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	// Action text - make it more visible
	actionText := fmt.Sprintf("Then %s %s %s",
		trade.Action.Type,
		trade.Action.Amount.String(),
		trade.Action.Pair)

	actionLabel := widget.NewLabel(actionText)

	// Add creation time and/or execution time if available
	timeInfo := fmt.Sprintf("Created: %s", trade.CreatedAt.Format("Jan 2 15:04:05"))
	if trade.ExecutedAt != nil {
		timeInfo += fmt.Sprintf(" | Executed: %s", trade.ExecutedAt.Format("Jan 2 15:04:05"))
	}
	timeLabel := widget.NewLabelWithStyle(timeInfo, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

	// Delete button with icon
	deleteButton := widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() {
		// Show confirmation dialog before deleting
		dialog.ShowConfirm("Delete Condition",
			"Are you sure you want to delete this condition?",
			func(confirmed bool) {
				if confirmed {
					b.deleteTrade(trade.ID)
				}
			},
			b.window)
	})

	// Visual separator
	separator := widget.NewSeparator()

	// Assemble card with better spacing
	content := container.NewVBox(
		container.NewPadded(conditionLabel),
		container.NewPadded(actionLabel),
		separator,
		container.NewHBox(
			statusContainer,
			layout.NewSpacer(), // This pushes the delete button to the right
			deleteButton,
		),
		timeLabel,
	)

	// Use a card with a border for better visibility
	card := widget.NewCard("", "", content)

	// Add some color based on status
	// Note: This would require custom rendering, but we can suggest this approach
	// A simpler alternative is to use different card titles based on status

	return card
}

// Update the refreshTradesDisplay function for better layout
func (b *ConditionalBotScreen) refreshTradesDisplay() {
	b.tradesContainer.Objects = nil

	if len(b.trades) == 0 {
		noConditionsLabel := widget.NewLabelWithStyle(
			"No conditions added yet.",
			fyne.TextAlignCenter,
			fyne.TextStyle{Italic: true},
		)
		b.tradesContainer.Add(noConditionsLabel)
	} else {
		// First add active/pending conditions
		var activeConditions int
		for _, trade := range b.trades {
			if !trade.Active || trade.Action.Executed {
				continue // Skip inactive or executed trades for now
			}

			// Create a card for each active trade
			tradeCard := b.createTradeCard(trade)
			b.tradesContainer.Add(tradeCard)
			// Add some space between cards
			b.tradesContainer.Add(widget.NewSeparator())
			activeConditions++
		}

		// If there are executed conditions, add a header and then the executed conditions
		var hasExecuted bool
		for _, trade := range b.trades {
			if trade.Action.Executed {
				if !hasExecuted {
					// Add a header for executed conditions
					b.tradesContainer.Add(widget.NewLabelWithStyle(
						"Executed Conditions",
						fyne.TextAlignCenter,
						fyne.TextStyle{Bold: true},
					))
					hasExecuted = true
				}

				tradeCard := b.createTradeCard(trade)
				b.tradesContainer.Add(tradeCard)
				// Add some space between cards
				b.tradesContainer.Add(widget.NewSeparator())
			}
		}

		// If there are inactive conditions, add a header and then the inactive conditions
		var hasInactive bool
		for _, trade := range b.trades {
			if !trade.Active && !trade.Action.Executed {
				if !hasInactive {
					// Add a header for inactive conditions
					b.tradesContainer.Add(widget.NewLabelWithStyle(
						"Inactive Conditions",
						fyne.TextAlignCenter,
						fyne.TextStyle{Bold: true},
					))
					hasInactive = true
				}

				tradeCard := b.createTradeCard(trade)
				b.tradesContainer.Add(tradeCard)
				// Add some space between cards
				b.tradesContainer.Add(widget.NewSeparator())
			}
		}

		if activeConditions == 0 && !hasExecuted && !hasInactive {
			// This case shouldn't happen but just in case
			b.tradesContainer.Add(widget.NewLabel("No visible conditions."))
		}
	}

	b.tradesContainer.Refresh()
}

// Delete a trade condition
func (b *ConditionalBotScreen) deleteTrade(id string) {
	// Remove from activeTrades map
	b.activeTrades.Delete(id)

	// Remove from trades list
	for i, trade := range b.trades {
		if trade.ID == id {
			b.trades = append(b.trades[:i], b.trades[i+1:]...)
			break
		}
	}

	b.logMessage(fmt.Sprintf("Deleted condition with ID: %s", id))
	b.refreshTradesDisplay()
}

// Toggle bot on/off
func (b *ConditionalBotScreen) toggleBot() {
	if b.isRunning {
		b.stopBot()
	} else {
		b.startBot()
	}
}

// Start the bot
func (b *ConditionalBotScreen) startBot() {
	// Check if wallet is loaded
	if b.fromAccount == nil {
		dialog.ShowError(fmt.Errorf("Please select and unlock a wallet first"), b.window)
		return
	}

	// Check if there are any active conditions
	if len(b.trades) == 0 {
		dialog.ShowError(fmt.Errorf("Please add at least one condition before starting"), b.window)
		return
	}

	b.isRunning = true
	b.status.SetText("Bot Status: Running")
	b.startStopButton.SetText("Stop Bot")
	b.logMessage("Bot started. Monitoring price conditions...")

	go b.runBot()
}

// Stop the bot
func (b *ConditionalBotScreen) stopBot() {
	b.isRunning = false
	b.status.SetText("Bot Status: Stopped")
	b.startStopButton.SetText("Start Bot")
	b.logMessage("Bot stopped.")
}

// Run the bot monitoring loop
func (b *ConditionalBotScreen) runBot() {
	for b.isRunning {
		b.checkConditions()
		time.Sleep(time.Duration(CNDTNL_CHECK_INTERVAL) * time.Second)
	}
}

// Check all conditions against current prices
func (b *ConditionalBotScreen) checkConditions() {
	// Fetch current prices
	prices, err := b.getPrices()
	if err != nil {
		b.logMessage(fmt.Sprintf("Error fetching prices: %v", err))
		return
	}

	// Log current prices
	b.logMessage("\nCurrent prices:")
	for asset, price := range prices {
		b.logMessage(fmt.Sprintf("%s: $%s", asset, price.String()))
	}

	// Check each active trade condition
	b.activeTrades.Range(func(key, value interface{}) bool {
		trade, ok := value.(*ConditionalTrade)
		if !ok || !trade.Active || trade.Action.Executed {
			return true // continue
		}

		// Get current price for the asset
		price, exists := prices[trade.Condition.Asset]
		if !exists {
			b.logMessage(fmt.Sprintf("No price data available for %s", trade.Condition.Asset))
			return true // continue
		}

		// Check if the condition is met
		conditionMet := b.evaluateCondition(trade.Condition, price)

		if conditionMet && !trade.Condition.Triggered {
			b.logMessage(fmt.Sprintf("Condition triggered for %s: %s price %s $%s (Current: $%s)",
				trade.ID,
				trade.Condition.Asset,
				simplifyOperator(trade.Condition.Operator),
				trade.Condition.Price.String(),
				price.String()))

			trade.Condition.Triggered = true

			// Execute action
			go b.executeAction(trade, prices)
		}

		return true
	})
}

// Evaluate if a condition is met
func (b *ConditionalBotScreen) evaluateCondition(condition PriceCondition, currentPrice decimal.Decimal) bool {
	switch condition.Operator {
	case "Greater Than (>)":
		return currentPrice.GreaterThan(condition.Price)
	case "Less Than (<)":
		return currentPrice.LessThan(condition.Price)
	case "Equal To (=)":
		return currentPrice.Equal(condition.Price)
	case "Greater Than or Equal (>=)":
		return currentPrice.GreaterThanOrEqual(condition.Price)
	case "Less Than or Equal (<=)":
		return currentPrice.LessThanOrEqual(condition.Price)
	default:
		return false
	}
}

// Execute the action for a triggered condition
func (b *ConditionalBotScreen) executeAction(trade *ConditionalTrade, prices map[string]decimal.Decimal) {
	b.logMessage(fmt.Sprintf("Executing action for condition %s: %s %s %s",
		trade.ID,
		trade.Action.Type,
		trade.Action.Amount.String(),
		trade.Action.Pair))

	// Variables for tracking transactions and errors
	var err error
	var swapTx *solana.Transaction
	var tipTx *solana.Transaction

	// Create the tip transaction first
	tipTx, err = b.createTipTransaction()
	if err != nil {
		b.logMessage(fmt.Sprintf("Error creating tip transaction: %v", err))
		// Continue with main transaction even if tip fails
	} else {
		b.logMessage("Tip transaction created successfully")
	}

	// Create the main transaction based on action type
	switch trade.Action.Type {
	case "Buy":
		swapTx, err = b.executeBuyTx(trade, prices)
	case "Sell":
		swapTx, err = b.executeSellTx(trade, prices)
	case "Send":
		swapTx, err = b.executeSendTx(trade)
	default:
		err = fmt.Errorf("Unsupported action type: %s", trade.Action.Type)
	}

	if err != nil {
		b.logMessage(fmt.Sprintf("Error creating main transaction: %v", err))
		return
	}

	b.logMessage("Main transaction created successfully")

	// If we have both transactions, try to bundle them
	if swapTx != nil && tipTx != nil {
		b.logMessage("Attempting to send transaction bundle...")
		bundleID, err := b.sendBundle([]*solana.Transaction{swapTx, tipTx})

		if err != nil {
			b.logMessage(fmt.Sprintf("Failed to send bundle: %v", err))
			b.logMessage("Falling back to sending individual transactions...")

			// Fall back to sending just the main transaction
			err = b.sendTransaction(swapTx)
			if err != nil {
				b.logMessage(fmt.Sprintf("Error sending main transaction: %v", err))
				return
			}
		} else {
			b.logMessage(fmt.Sprintf("Bundle sent successfully with ID: %s", bundleID))
		}
	} else if swapTx != nil {
		// If we only have the main transaction, send it directly
		b.logMessage("Sending main transaction only...")
		err = b.sendTransaction(swapTx)
		if err != nil {
			b.logMessage(fmt.Sprintf("Error sending main transaction: %v", err))
			return
		}
	} else {
		b.logMessage("No valid transactions to send")
		return
	}

	// Mark as executed
	trade.Action.Executed = true
	now := time.Now()
	trade.ExecutedAt = &now

	b.logMessage(fmt.Sprintf("Action executed successfully for condition %s", trade.ID))

	// Refresh display
	b.refreshTradesDisplay()
}

// sendTransaction sends a single transaction using RPC
func (b *ConditionalBotScreen) sendTransaction(tx *solana.Transaction) error {
	// Convert transaction to wire format
	serializedTx, err := tx.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to serialize transaction: %v", err)
	}

	// Set up the RPC request manually
	rpcRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "sendTransaction",
		"params": []interface{}{
			base58.Encode(serializedTx),
			map[string]interface{}{
				"encoding":            "base58",
				"skipPreflight":       true, // Skip preflight to avoid simulation errors
				"preflightCommitment": "confirmed",
				"maxRetries":          5,
			},
		},
	}

	reqBody, err := json.Marshal(rpcRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal RPC request: %v", err)
	}

	// Log the request we're about to send
	b.logMessage(fmt.Sprintf("Sending RPC request: %s", string(reqBody)))

	// Send the request manually
	resp, err := http.Post(CNDTNL_ENDPOINT, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("failed to send RPC request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}

	b.logMessage(fmt.Sprintf("RPC response: %s", string(respBody)))

	// Parse the response
	var rpcResponse struct {
		Result string                 `json:"result"`
		Error  map[string]interface{} `json:"error"`
	}

	if err := json.Unmarshal(respBody, &rpcResponse); err != nil {
		return fmt.Errorf("failed to parse response: %v", err)
	}

	if rpcResponse.Error != nil {
		return fmt.Errorf("RPC error: %v", rpcResponse.Error)
	}

	b.logMessage(fmt.Sprintf("Transaction sent successfully with signature: %s", rpcResponse.Result))
	return nil
}

// executeBuyTx creates a buy transaction
func (b *ConditionalBotScreen) executeBuyTx(trade *ConditionalTrade, prices map[string]decimal.Decimal) (*solana.Transaction, error) {
	// Parse trading pair
	pair, ok := TradingPairs[trade.Action.Pair]
	if !ok {
		return nil, fmt.Errorf("Unsupported trading pair: %s", trade.Action.Pair)
	}

	// For buying, input is the quote currency (e.g., USDC)
	inputMint := pair.QuoteMint
	outputMint := pair.BaseMint

	// Calculate input amount (in lamports/smallest units)
	// Amount is in quote currency (e.g., USDC)
	amountLamports := trade.Action.Amount.Mul(decimal.New(1, int32(pair.QuoteDecimals))).IntPart()

	// Log the swap details
	b.logMessage(fmt.Sprintf("Executing buy: %s %s with %s %s (Amount in lamports: %d)",
		trade.Action.Amount, pair.BaseSymbol, trade.Action.Amount, pair.QuoteSymbol, amountLamports))

	// Get swap instructions
	swapInstructions, err := b.getJupiterSwapInstructions(
		b.fromAccount.PublicKey(),
		inputMint,
		outputMint,
		amountLamports,
		100, // 1% slippage
	)
	if err != nil {
		return nil, fmt.Errorf("Error getting swap instructions: %v", err)
	}

	// Check for errors in the response
	if errorVal, ok := swapInstructions["error"]; ok {
		return nil, fmt.Errorf("Jupiter API error: %v", errorVal)
	}

	// Get latest blockhash
	recentBlockhash, err := b.client.GetLatestBlockhash(context.Background(), rpc.CommitmentFinalized)
	if err != nil {
		b.logMessage(fmt.Sprintf("Failed to get latest blockhash: %v", err))
		return nil, fmt.Errorf("failed to get latest blockhash: %v", err)
	}
	b.logMessage(fmt.Sprintf("Latest blockhash: %s", recentBlockhash.Value.Blockhash))

	// Create an array of instructions
	var instructions []solana.Instruction

	// Add compute budget instruction if present
	computeBudgetInstructions, hasBudget := swapInstructions["computeBudgetInstructions"].([]interface{})
	if hasBudget {
		for i, instruction := range computeBudgetInstructions {
			instData, ok := instruction.(map[string]interface{})
			if !ok {
				b.logMessage(fmt.Sprintf("Error: compute budget instruction %d is not of expected type", i))
				continue
			}
			instructions = append(instructions, b.createTransactionInstruction(instData))
		}
		b.logMessage(fmt.Sprintf("Added %d compute budget instructions", len(computeBudgetInstructions)))
	}

	// Add setup instructions
	setupInstructions, hasSetup := swapInstructions["setupInstructions"].([]interface{})
	if hasSetup {
		for i, instruction := range setupInstructions {
			instData, ok := instruction.(map[string]interface{})
			if !ok {
				b.logMessage(fmt.Sprintf("Error: setup instruction %d is not of expected type", i))
				continue
			}
			instructions = append(instructions, b.createTransactionInstruction(instData))
		}
		b.logMessage(fmt.Sprintf("Added %d setup instructions", len(setupInstructions)))
	}

	// Add main swap instruction
	swapInstruction, ok := swapInstructions["swapInstruction"].(map[string]interface{})
	if !ok {
		b.logMessage("Error: swapInstruction is not of expected type")
		return nil, fmt.Errorf("swapInstruction is not of expected type")
	}
	instructions = append(instructions, b.createTransactionInstruction(swapInstruction))
	b.logMessage("Added main swap instruction")

	// Add cleanup instruction if present
	if cleanupInst, ok := swapInstructions["cleanupInstruction"]; ok && cleanupInst != nil {
		cleanupInstData, ok := cleanupInst.(map[string]interface{})
		if !ok {
			b.logMessage("Error: cleanupInstruction is not of expected type")
		} else {
			instructions = append(instructions, b.createTransactionInstruction(cleanupInstData))
			b.logMessage("Added cleanup instruction")
		}
	}

	// Create the transaction with all instructions
	tx, err := solana.NewTransaction(
		instructions,
		recentBlockhash.Value.Blockhash,
		solana.TransactionPayer(b.fromAccount.PublicKey()),
	)
	if err != nil {
		b.logMessage(fmt.Sprintf("Failed to create transaction: %v", err))
		return nil, fmt.Errorf("failed to create transaction: %v", err)
	}

	// Sign transaction
	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(b.fromAccount.PublicKey()) {
			return b.fromAccount
		}
		return nil
	})
	if err != nil {
		b.logMessage(fmt.Sprintf("Failed to sign transaction: %v", err))
		return nil, fmt.Errorf("failed to sign transaction: %v", err)
	}
	b.logMessage(fmt.Sprintf("Transaction created and signed with signature: %s", tx.Signatures[0]))

	return tx, nil
}

// executeSellTx creates a sell transaction
func (b *ConditionalBotScreen) executeSellTx(trade *ConditionalTrade, prices map[string]decimal.Decimal) (*solana.Transaction, error) {
	// Parse trading pair
	pair, ok := TradingPairs[trade.Action.Pair]
	if !ok {
		return nil, fmt.Errorf("Unsupported trading pair: %s", trade.Action.Pair)
	}

	// For selling, input is the base currency (e.g., SOL)
	inputMint := pair.BaseMint
	outputMint := pair.QuoteMint

	// Calculate input amount (in lamports/smallest units)
	// Amount is in base currency (e.g., SOL)
	amountLamports := trade.Action.Amount.Mul(decimal.New(1, int32(pair.BaseDecimals))).IntPart()

	// Log the swap details
	b.logMessage(fmt.Sprintf("Executing sell: %s %s for %s (Amount in lamports: %d)",
		trade.Action.Amount, pair.BaseSymbol, pair.QuoteSymbol, amountLamports))

	// Get swap instructions
	swapInstructions, err := b.getJupiterSwapInstructions(
		b.fromAccount.PublicKey(),
		inputMint,
		outputMint,
		amountLamports,
		100, // 1% slippage
	)
	if err != nil {
		return nil, fmt.Errorf("Error getting swap instructions: %v", err)
	}

	// Check for errors in the response
	if errorVal, ok := swapInstructions["error"]; ok {
		return nil, fmt.Errorf("Jupiter API error: %v", errorVal)
	}

	// Get latest blockhash
	recentBlockhash, err := b.client.GetLatestBlockhash(context.Background(), rpc.CommitmentFinalized)
	if err != nil {
		b.logMessage(fmt.Sprintf("Failed to get latest blockhash: %v", err))
		return nil, fmt.Errorf("failed to get latest blockhash: %v", err)
	}
	b.logMessage(fmt.Sprintf("Latest blockhash: %s", recentBlockhash.Value.Blockhash))

	// Create an array of instructions
	var instructions []solana.Instruction

	// Add compute budget instruction if present
	computeBudgetInstructions, hasBudget := swapInstructions["computeBudgetInstructions"].([]interface{})
	if hasBudget {
		for i, instruction := range computeBudgetInstructions {
			instData, ok := instruction.(map[string]interface{})
			if !ok {
				b.logMessage(fmt.Sprintf("Error: compute budget instruction %d is not of expected type", i))
				continue
			}
			instructions = append(instructions, b.createTransactionInstruction(instData))
		}
		b.logMessage(fmt.Sprintf("Added %d compute budget instructions", len(computeBudgetInstructions)))
	}

	// Add setup instructions
	setupInstructions, hasSetup := swapInstructions["setupInstructions"].([]interface{})
	if hasSetup {
		for i, instruction := range setupInstructions {
			instData, ok := instruction.(map[string]interface{})
			if !ok {
				b.logMessage(fmt.Sprintf("Error: setup instruction %d is not of expected type", i))
				continue
			}
			instructions = append(instructions, b.createTransactionInstruction(instData))
		}
		b.logMessage(fmt.Sprintf("Added %d setup instructions", len(setupInstructions)))
	}

	// Add main swap instruction
	swapInstruction, ok := swapInstructions["swapInstruction"].(map[string]interface{})
	if !ok {
		b.logMessage("Error: swapInstruction is not of expected type")
		return nil, fmt.Errorf("swapInstruction is not of expected type")
	}
	instructions = append(instructions, b.createTransactionInstruction(swapInstruction))
	b.logMessage("Added main swap instruction")

	// Add cleanup instruction if present
	if cleanupInst, ok := swapInstructions["cleanupInstruction"]; ok && cleanupInst != nil {
		cleanupInstData, ok := cleanupInst.(map[string]interface{})
		if !ok {
			b.logMessage("Error: cleanupInstruction is not of expected type")
		} else {
			instructions = append(instructions, b.createTransactionInstruction(cleanupInstData))
			b.logMessage("Added cleanup instruction")
		}
	}

	// Create the transaction with all instructions
	tx, err := solana.NewTransaction(
		instructions,
		recentBlockhash.Value.Blockhash,
		solana.TransactionPayer(b.fromAccount.PublicKey()),
	)
	if err != nil {
		b.logMessage(fmt.Sprintf("Failed to create transaction: %v", err))
		return nil, fmt.Errorf("failed to create transaction: %v", err)
	}

	// Sign transaction
	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(b.fromAccount.PublicKey()) {
			return b.fromAccount
		}
		return nil
	})
	if err != nil {
		b.logMessage(fmt.Sprintf("Failed to sign transaction: %v", err))
		return nil, fmt.Errorf("failed to sign transaction: %v", err)
	}
	b.logMessage(fmt.Sprintf("Transaction created and signed with signature: %s", tx.Signatures[0]))

	return tx, nil
}

// executeSendTx creates a send transaction (placeholder for future implementation)
func (b *ConditionalBotScreen) executeSendTx(trade *ConditionalTrade) (*solana.Transaction, error) {
	// For MVP, we'll just log this action
	b.logMessage("Send functionality not fully implemented in MVP")
	b.logMessage(fmt.Sprintf("Would send %s of %s",
		trade.Action.Amount.String(),
		strings.Split(trade.Action.Pair, "/")[0]))

	return nil, fmt.Errorf("Send transaction not fully implemented")
}

func (b *ConditionalBotScreen) createTransactionInstruction(instructionData map[string]interface{}) solana.Instruction {
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

// createTipTransaction creates a transaction that sends tips to a Jito tip account
func (b *ConditionalBotScreen) createTipTransaction() (*solana.Transaction, error) {
	b.logMessage("Creating tip transaction...")

	// Get latest blockhash
	recentBlockhash, err := b.client.GetLatestBlockhash(context.Background(), rpc.CommitmentFinalized)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest blockhash: %v", err)
	}

	// Create transaction builder
	builder := solana.NewTransactionBuilder()
	builder.SetFeePayer(b.fromAccount.PublicKey())
	builder.SetRecentBlockHash(recentBlockhash.Value.Blockhash)

	// These are the 8 Jito tip accounts
	jitoTipAccounts := []string{
		"96gYZGLnJYVFmbjzopPSU6QiEV5fGqZNyN9nmNhvrZU5",
		"Gd6dV4ESSHcQAMWJpZ9mX6mGCQ1jNEKRDz3Q58wp3Wx3",
		"DttWaMuVvTiduZRnguLF7jNxTgiMBZ1hyAumKUiL2KRL",
		"4nh6f8rJjbP5eYbCvQ3rd5EVFFmZ4nYvRB34mFAEenHJ",
		"3AVi9Tg9Uo68tJfuvoKvqKNWKkC5wPdSSdeBnizKzYCN",
		"EZknHQUZ6YByhGxXia8MKKhu39LzjVV7NLd5xuVwmz7R",
		"6SCu87GnSgHgxNsUoVjdLnkgJoM5VLUiitKUa7xHKbys",
		"JC4sWJYkueeRYgwwMvCTi4M311ZWmJKptZ1T8p7vBqKr",
	}

	// Choose one of the tip accounts at random to reduce contention
	tipIndex := time.Now().UnixNano() % int64(len(jitoTipAccounts))
	tipAccount := jitoTipAccounts[tipIndex]

	// Send a substantial tip (0.005 SOL) to make sure our bundle gets processed
	// 5,000,000 lamports = 0.005 SOL
	tipAmount := uint64(5_000_000)

	tipInstruction := system.NewTransferInstruction(
		tipAmount,
		b.fromAccount.PublicKey(),
		solana.MustPublicKeyFromBase58(tipAccount),
	).Build()

	builder.AddInstruction(tipInstruction)

	// Optional: Also add tip to Unruggable account
	builder.AddInstruction(system.NewTransferInstruction(
		1_000_000, // 0.001 SOL
		b.fromAccount.PublicKey(),
		solana.MustPublicKeyFromBase58("juLesoSmdTcRtzjCzYzRoHrnF8GhVu6KCV7uxq7nJGp"),
	).Build())

	// Build the transaction
	tx, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build tip transaction: %v", err)
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

	b.logMessage(fmt.Sprintf("Tip transaction created and signed with signature: %s", tx.Signatures[0]))
	b.logMessage(fmt.Sprintf("Sending tip of %d lamports to Jito account %s", tipAmount, tipAccount))

	return tx, nil
}

// Updated getJupiterSwapInstructions function to fix the fee parameter conflict
func (b *ConditionalBotScreen) getJupiterSwapInstructions(fromAccountPublicKey solana.PublicKey, inputMint, outputMint string, amountLamports int64, slippageBps int) (map[string]interface{}, error) {
	b.logMessage(fmt.Sprintf("Getting Jupiter swap instructions for %s to %s...", inputMint, outputMint))

	// Updated URL structure for Jupiter v6 API
	quoteURL := fmt.Sprintf("%s?inputMint=%s&outputMint=%s&amount=%d&slippageBps=%d&onlyDirectRoutes=false",
		CNDTNL_JUPITER_QUOTE_URL, inputMint, outputMint, amountLamports, slippageBps)

	b.logMessage(fmt.Sprintf("Requesting quote from: %s", quoteURL))
	quoteResp, err := http.Get(quoteURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get Jupiter quote: %v", err)
	}
	defer quoteResp.Body.Close()

	quoteBody, err := ioutil.ReadAll(quoteResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read quote response: %v", err)
	}
	b.logMessage(fmt.Sprintf("Quote response: %s", string(quoteBody)))

	var quoteData map[string]interface{}
	if err := json.Unmarshal(quoteBody, &quoteData); err != nil {
		return nil, fmt.Errorf("failed to decode Jupiter quote: %v", err)
	}

	// Set up the swap instructions request
	// FIXED: Only using prioritizationFeeLamports, not both fee parameters
	swapBody := map[string]interface{}{
		"userPublicKey":             fromAccountPublicKey.String(),
		"quoteResponse":             quoteData,
		"wrapAndUnwrapSol":          true,
		"useSharedAccounts":         false,   // Disable shared accounts to avoid the panic
		"prioritizationFeeLamports": 1000000, // Only using priority fee
		"dynamicComputeUnitLimit":   true,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(swapBody); err != nil {
		return nil, fmt.Errorf("failed to encode swap body: %v", err)
	}

	b.logMessage(fmt.Sprintf("Requesting swap instructions with payload: %s", buf.String()))
	swapResp, err := http.Post(CNDTNL_JUPITER_SWAP_INSTR, "application/json", bytes.NewBuffer(buf.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("failed to get swap instructions: %v", err)
	}
	defer swapResp.Body.Close()

	// Read the response body as bytes
	respBody, err := ioutil.ReadAll(swapResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read swap instructions response: %v", err)
	}
	b.logMessage(fmt.Sprintf("Swap instructions response: %s", string(respBody)))

	// Parse the JSON response into a map
	var swapData map[string]interface{}
	if err := json.Unmarshal(respBody, &swapData); err != nil {
		return nil, fmt.Errorf("failed to decode swap instructions: %v", err)
	}

	b.logMessage("Jupiter swap instructions fetched successfully")
	return swapData, nil
}

// getPrices fetches prices from Jupiter API
func (b *ConditionalBotScreen) getPrices() (map[string]decimal.Decimal, error) {
	// Create a map to store the results
	prices := make(map[string]decimal.Decimal)

	// Set USDC price as reference (always 1.0)
	prices["USDC"] = decimal.NewFromFloat(1.0)

	// Collect mint addresses for all tokens except USDC
	var mintAddresses []string
	symbolToMint := make(map[string]string)
	mintToSymbol := make(map[string]string)

	for symbol, asset := range MonitoredAssets {
		if symbol != "USDC" { // Skip USDC since we already set it
			mintAddresses = append(mintAddresses, asset.TokenMint)
			symbolToMint[symbol] = asset.TokenMint
			mintToSymbol[asset.TokenMint] = symbol
		}
	}

	// Build the URL
	baseURL, err := url.Parse(CNDTNL_JUPITER_PRICE_URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %v", err)
	}

	// Construct query params - Jupiter API expects mint addresses
	params := url.Values{}
	params.Add("ids", strings.Join(mintAddresses, ","))
	baseURL.RawQuery = params.Encode()

	// Log the URL we're requesting
	b.logMessage(fmt.Sprintf("Requesting prices from: %s", baseURL.String()))

	// Create a client with timeout
	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	// Make the HTTP request
	resp, err := client.Get(baseURL.String())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch prices: %v", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	// Read response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// Log the response for debugging
	if len(body) > 0 {
		previewLen := 300
		if len(body) < previewLen {
			previewLen = len(body)
		}
		b.logMessage(fmt.Sprintf("Response preview: %s", string(body[:previewLen])))
	} else {
		b.logMessage("Empty response body received")
		return nil, fmt.Errorf("empty response from price API")
	}

	// Parse the JSON response
	var jupiterResp struct {
		Data map[string]struct {
			ID    string `json:"id"`
			Type  string `json:"type"`
			Price string `json:"price"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &jupiterResp); err != nil {
		return nil, fmt.Errorf("failed to decode price response: %v", err)
	}

	// Convert prices to decimals, mapping mint addresses back to symbols
	for mint, data := range jupiterResp.Data {
		symbol, ok := mintToSymbol[mint]
		if !ok {
			b.logMessage(fmt.Sprintf("Unrecognized mint address in response: %s", mint))
			continue
		}

		price, err := decimal.NewFromString(data.Price)
		if err != nil {
			b.logMessage(fmt.Sprintf("Could not parse price for %s (%s): %v", symbol, mint, err))
			continue
		}

		prices[symbol] = price
		b.logMessage(fmt.Sprintf("Price for %s: $%s", symbol, prices[symbol].String()))
	}

	// Check that we have prices for all required assets
	for symbol := range MonitoredAssets {
		if symbol != "USDC" && !prices[symbol].IsPositive() {
			b.logMessage(fmt.Sprintf("Missing price for %s", symbol))
		}
	}

	return prices, nil
}

// sendBundle sends a bundle of transactions to Jito
func (b *ConditionalBotScreen) sendBundle(transactions []*solana.Transaction) (string, error) {
	b.logMessage("Preparing transaction bundle...")

	// Validate transactions
	if len(transactions) == 0 {
		return "", fmt.Errorf("no transactions to send")
	}

	// Check if we have more than 5 transactions (Jito limit)
	if len(transactions) > 5 {
		return "", fmt.Errorf("bundle exceeds maximum of 5 transactions")
	}

	// Encode transactions to base64 (preferred for performance)
	encodedTransactions := make([]string, len(transactions))
	for i, tx := range transactions {
		// Make sure the transaction is signed
		if len(tx.Signatures) == 0 {
			return "", fmt.Errorf("transaction %d is not signed", i)
		}

		// Marshal the transaction to binary
		encodedTx, err := tx.MarshalBinary()
		if err != nil {
			return "", fmt.Errorf("failed to encode transaction %d: %v", i, err)
		}

		// Encode the binary to base64
		encodedTransactions[i] = base64.StdEncoding.EncodeToString(encodedTx)
		b.logMessage(fmt.Sprintf("Encoded transaction %d with signature: %s",
			i+1, tx.Signatures[0].String()))
	}

	// Create the bundle request with correct structure
	// IMPORTANT: The params field is an array containing:
	// 1. An array of transaction strings
	// 2. An optional object with encoding details
	bundleData := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "sendBundle",
		"params": []interface{}{
			encodedTransactions,
			map[string]string{
				"encoding": "base64",
			},
		},
	}

	// Marshal to JSON
	bundleJSON, err := json.Marshal(bundleData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal bundle data: %v", err)
	}

	// Log the request payload for debugging
	b.logMessage(fmt.Sprintf("Bundle request payload: %s", string(bundleJSON)))

	// Send the request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(
		CNDTNL_JITO_BUNDLE_URL,
		"application/json",
		bytes.NewBuffer(bundleJSON),
	)
	if err != nil {
		return "", fmt.Errorf("failed to send bundle: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	// Log the raw response
	b.logMessage(fmt.Sprintf("Raw bundle response: %s", string(respBody)))

	// Parse the response
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to decode bundle response: %v", err)
	}

	// Check for errors
	if errorData, ok := result["error"]; ok {
		errorMsg := "unknown error"
		if errObj, ok := errorData.(map[string]interface{}); ok {
			if msg, ok := errObj["message"].(string); ok {
				errorMsg = msg
			}
		}
		return "", fmt.Errorf("bundle error: %s", errorMsg)
	}

	// Extract bundle ID
	bundleID, ok := result["result"].(string)
	if !ok {
		return "", fmt.Errorf("invalid bundle response format")
	}

	b.logMessage(fmt.Sprintf("Bundle successfully sent with ID: %s", bundleID))
	return bundleID, nil
}

// Helper function for logging
func (b *ConditionalBotScreen) logMessage(message string) {
	log.Println(message)
	b.log.SetText(b.log.Text + message + "\n")

	// Scroll to the bottom
	b.log.CursorRow = len(strings.Split(b.log.Text, "\n")) - 1
	b.log.Refresh()
}
