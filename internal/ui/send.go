package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"unruggable-go/internal/storage"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/validation"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/gagliardetto/solana-go"
	associatedtokenaccount "github.com/gagliardetto/solana-go/programs/associated-token-account"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/programs/token"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/mr-tron/base58"
)

// Using a public Solana RPC endpoint as fallback
const (
	SOLANA_RPC_ENDPOINT = "https://special-blue-fog.solana-mainnet.quiknode.pro/d009d548b4b9dd9f062a8124a868fb915937976c/"
	CALYPSO_ENDPOINT    = "https://special-blue-fog.solana-mainnet.quiknode.pro/d009d548b4b9dd9f062a8124a868fb915937976c/"
	JITO_URL            = "https://mainnet.block-engine.jito.wtf/api/v1/bundles"
)

type SendScreen struct {
	container        *fyne.Container
	tokenSelect      *widget.Select
	amountEntry      *widget.Entry
	recipientEntry   *widget.Entry
	recipientBalance *widget.Label
	sendButton       *widget.Button
	refreshButton    *widget.Button
	statusLabel      *widget.Label
	window           fyne.Window
	client           *rpc.Client
	fromAccount      *solana.PrivateKey
	app              fyne.App
	selectedWalletID string
	isLoadingBalance bool
	isVerboseLogging bool // Add this line
}

// Direct RPC request structure for getBalance
type RpcRequest struct {
	JsonRPC string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

// Direct RPC response structure for getBalance
type RpcResponse struct {
	JsonRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  struct {
		Context struct {
			Slot int64 `json:"slot"`
		} `json:"context"`
		Value uint64 `json:"value"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func NewSendScreen(window fyne.Window, app fyne.App) fyne.CanvasObject {
	s := &SendScreen{
		window:           window,
		app:              app,
		client:           rpc.New(CALYPSO_ENDPOINT),
		recipientBalance: widget.NewLabel(""),
		statusLabel:      widget.NewLabel(""),
		isLoadingBalance: false,
	}

	// Get the globally selected wallet
	selectedWallet := GetGlobalState().GetSelectedWallet()
	if selectedWallet != "" {
		s.selectedWalletID = selectedWallet
		s.statusLabel.SetText(fmt.Sprintf("Using wallet: %s", shortenAddress(selectedWallet)))
	} else {
		s.statusLabel.SetText("No wallet selected. Please select a wallet from the Wallet tab.")
	}

	// Token selection
	tokenOptions := s.getTokenOptions()
	s.tokenSelect = widget.NewSelect(tokenOptions, s.onTokenSelected)
	s.tokenSelect.PlaceHolder = "Select token to send"

	// Amount entry with validation
	s.amountEntry = widget.NewEntry()
	s.amountEntry.SetPlaceHolder("Enter amount")
	s.amountEntry.Validator = validation.NewRegexp(`^[0-9]*\.?[0-9]*$`, "Must be a valid number")
	s.amountEntry.OnChanged = func(text string) {
		s.validateForm()
	}

	// Max button
	maxButton := widget.NewButton("Max", func() {
		balances := GetGlobalState().GetWalletBalances()
		if balances == nil || s.tokenSelect.Selected == "" {
			return
		}

		var availableBalance float64
		if s.tokenSelect.Selected == "SOL" {
			if balances.SolBalance > 0.01 {
				availableBalance = balances.SolBalance - 0.01
			}
		} else {
			for _, holding := range balances.Assets {
				if holding.Symbol == s.tokenSelect.Selected {
					availableBalance = holding.Balance
					break
				}
			}
		}

		if availableBalance > 0 {
			s.amountEntry.SetText(fmt.Sprintf("%.9f", availableBalance))
		}
	})
	maxButton.Importance = widget.MediumImportance
	maxButton.Resize(fyne.NewSize(60, maxButton.MinSize().Height))

	// Recipient address entry with validation
	s.recipientEntry = widget.NewEntry()
	s.recipientEntry.SetPlaceHolder("Enter recipient's Solana address")
	s.recipientEntry.OnChanged = s.validateAndFetchBalance
	s.recipientEntry.Validator = validation.NewRegexp(`^[1-9A-HJ-NP-Za-km-z]{43,44}$`, "Must be a valid Solana address")

	// Send button
	s.sendButton = widget.NewButton("Send", s.handleSendTransaction)
	s.sendButton.Importance = widget.HighImportance
	s.sendButton.Disable()

	// Refresh balances button
	s.refreshButton = widget.NewButton("Refresh", func() {
		s.refreshWalletBalances()
	})

	// Layout with compact design
	form := container.NewVBox(
		// Token row with balance
		container.NewGridWithColumns(2,
			widget.NewLabel("Token:"),
			s.tokenSelect,
		),

		// Amount row with Max button
		container.NewBorder(nil, nil, nil, maxButton,
			container.NewGridWithColumns(2,
				widget.NewLabel("Amount:"),
				s.amountEntry,
			),
		),

		// Recipient row
		container.NewGridWithColumns(2,
			widget.NewLabel("Recipient:"),
			s.recipientEntry,
		),

		// Recipient balance info
		container.NewHBox(
			widget.NewIcon(theme.InfoIcon()),
			s.recipientBalance,
		),

		// Action buttons in a row
		container.NewGridWithColumns(2,
			s.sendButton,
			s.refreshButton,
		),

		// Status message
		container.NewHBox(
			widget.NewIcon(theme.InfoIcon()),
			s.statusLabel,
		),
	)

	// Wrap in scroll container for mobile
	scroll := container.NewScroll(form)
	s.container = container.NewPadded(scroll)

	// Initialize with first token if available
	if len(tokenOptions) > 0 && GetGlobalState().GetWalletBalances() != nil {
		s.tokenSelect.SetSelected(tokenOptions[0])
	}

	return s.container
}

func (s *SendScreen) getTokenOptions() []string {
	balances := GetGlobalState().GetWalletBalances()
	if balances == nil {
		return []string{"SOL"} // Fallback to SOL if balances not loaded
	}
	options := []string{"SOL"}
	for _, asset := range balances.Assets {
		if asset.Balance > 0 { // Only show tokens with non-zero balance
			options = append(options, asset.Symbol)
		}
	}
	return options
}

func (s *SendScreen) onTokenSelected(value string) {
	if value != "" {
		s.validateForm()
		s.updateBalanceInfo()
	}
}

func (s *SendScreen) updateBalanceInfo() {
	if s.tokenSelect.Selected == "" {
		return
	}

	balances := GetGlobalState().GetWalletBalances()
	if balances == nil {
		return
	}

	var availableBalance float64
	var symbol string

	if s.tokenSelect.Selected == "SOL" {
		availableBalance = balances.SolBalance
		symbol = "SOL"
	} else {
		for _, holding := range balances.Assets {
			if holding.Symbol == s.tokenSelect.Selected {
				availableBalance = holding.Balance
				symbol = holding.Symbol
				break
			}
		}
	}

	// Update the status label with the balance info
	s.statusLabel.SetText(fmt.Sprintf("Available: %.6f %s", availableBalance, symbol))
}

// fetchBalanceWithRPC is a direct RPC call to get balance without using the solana-go client
func (s *SendScreen) fetchBalanceWithRPC(address string) {
	// Don't start a new fetch if one is already in progress
	if s.isLoadingBalance {
		return
	}

	s.isLoadingBalance = true
	s.recipientBalance.SetText("Checking recipient account...")

	go func() {
		defer func() { s.isLoadingBalance = false }()

		// Try multiple RPC endpoints
		endpoints := []string{CALYPSO_ENDPOINT, SOLANA_RPC_ENDPOINT}

		for _, endpoint := range endpoints {
			// Create RPC request
			reqBody := RpcRequest{
				JsonRPC: "2.0",
				ID:      1,
				Method:  "getBalance",
				Params:  []interface{}{address},
			}

			jsonData, err := json.Marshal(reqBody)
			if err != nil {
				continue // Try next endpoint
			}

			// Send request
			resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(jsonData))
			if err != nil {
				continue // Try next endpoint
			}
			defer resp.Body.Close()

			// Read and parse response
			respData, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				continue // Try next endpoint
			}

			var rpcResp RpcResponse
			if err := json.Unmarshal(respData, &rpcResp); err != nil {
				continue // Try next endpoint
			}

			// Check for errors
			if rpcResp.Error != nil {
				continue // Try next endpoint
			}

			// Success - update UI and return
			solBalance := float64(rpcResp.Result.Value) / float64(solana.LAMPORTS_PER_SOL)
			s.recipientBalance.SetText(fmt.Sprintf("Recipient SOL: %.6f", solBalance))
			return
		}

		// All endpoints failed
		s.recipientBalance.SetText("Could not fetch recipient balance")
	}()
}

func (s *SendScreen) validateAndFetchBalance(address string) {
	s.recipientBalance.SetText("")
	s.validateForm()

	if address == "" || len(address) < 32 || !isValidSolanaAddress(address) {
		return
	}

	// Use the direct RPC method instead of the client
	s.fetchBalanceWithRPC(address)
}

func (s *SendScreen) refreshWalletBalances() {
	s.statusLabel.SetText("Refreshing balances...")

	go func() {
		if err := RefreshWalletBalances(); err != nil {
			s.statusLabel.SetText(fmt.Sprintf("Refresh failed: %v", err))
			return
		}

		s.tokenSelect.Options = s.getTokenOptions()
		s.tokenSelect.Refresh()
		s.validateForm()
		s.updateBalanceInfo()
		s.window.Canvas().Refresh(s.container)
	}()
}

func isValidSolanaAddress(address string) bool {
	if len(address) != 44 && len(address) != 43 {
		return false
	}
	_, err := solana.PublicKeyFromBase58(address)
	return err == nil
}

func (s *SendScreen) validateForm() {
	// Guard against nil reference or empty selection
	if s.tokenSelect == nil || s.tokenSelect.Selected == "" || s.amountEntry == nil || s.recipientEntry == nil {
		if s.sendButton != nil {
			s.sendButton.Disable()
		}
		return
	}

	// If any field is empty, disable the button and return early
	if s.amountEntry.Text == "" || s.recipientEntry.Text == "" {
		s.sendButton.Disable()
		return
	}

	// Validate amount and recipient address
	if s.amountEntry.Validate() != nil || !isValidSolanaAddress(s.recipientEntry.Text) {
		s.sendButton.Disable()
		return
	}

	// Parse the amount
	amount, err := strconv.ParseFloat(s.amountEntry.Text, 64)
	if err != nil {
		s.sendButton.Disable()
		return
	}

	// Check wallet balances
	balances := GetGlobalState().GetWalletBalances()
	if balances == nil {
		s.sendButton.Disable()
		s.statusLabel.SetText("Balances not loaded yet. Please wait or refresh.")
		return
	}

	// Determine available balance for the selected token
	selectedToken := s.tokenSelect.Selected
	var availableBalance float64
	if selectedToken == "SOL" {
		availableBalance = balances.SolBalance
	} else {
		for _, holding := range balances.Assets {
			if holding.Symbol == selectedToken {
				availableBalance = holding.Balance
				break
			}
		}
	}

	// Check if the amount is valid and sufficient
	if amount <= 0 || amount > availableBalance {
		s.sendButton.Disable()
		s.statusLabel.SetText(fmt.Sprintf("Insufficient balance: %.6f %s available", availableBalance, selectedToken))
		return
	}

	// If all checks pass, enable the send button
	s.sendButton.Enable()
	s.updateBalanceInfo()
}

func (s *SendScreen) handleSendTransaction() {
	if s.selectedWalletID == "" {
		dialog.ShowError(fmt.Errorf("no wallet selected - please select a wallet first"), s.window)
		return
	}

	amount, err := strconv.ParseFloat(s.amountEntry.Text, 64)
	if err != nil {
		dialog.ShowError(fmt.Errorf("invalid amount"), s.window)
		return
	}

	// Confirm the transaction
	confirmText := fmt.Sprintf("Send %.6f %s to %s?",
		amount,
		s.tokenSelect.Selected,
		shortenAddress(s.recipientEntry.Text))

	dialog.ShowConfirm("Confirm Transaction", confirmText, func(confirmed bool) {
		if !confirmed {
			return
		}

		// Show password dialog for decryption
		passwordEntry := widget.NewPasswordEntry()
		passwordEntry.SetPlaceHolder("Enter wallet password")

		dialog.ShowCustomConfirm("Decrypt Wallet", "Send", "Cancel", passwordEntry, func(confirm bool) {
			if !confirm {
				return
			}

			// Decrypt wallet
			if err := s.decryptAndPrepareWallet(passwordEntry.Text); err != nil {
				dialog.ShowError(err, s.window)
				return
			}

			// Proceed with transaction
			go s.executeTransaction(amount)
		}, s.window)
	}, s.window)
}

func (s *SendScreen) decryptAndPrepareWallet(password string) error {
	// Use the storage abstraction to access wallet data
	walletStorage := storage.NewWalletStorage(s.app)
	walletMap, err := walletStorage.LoadWallets()
	if err != nil {
		return fmt.Errorf("error loading wallets: %v", err)
	}

	// Get the encrypted wallet data
	encryptedData, ok := walletMap[s.selectedWalletID]
	if !ok {
		return fmt.Errorf("wallet %s not found", s.selectedWalletID)
	}

	decryptedKey, err := decrypt(encryptedData, password)
	if err != nil {
		return fmt.Errorf("failed to decrypt wallet: %v", err)
	}

	privateKey := solana.MustPrivateKeyFromBase58(string(decryptedKey))
	s.fromAccount = &privateKey
	return nil
}

func (s *SendScreen) createTransferTransaction(fromWallet, toAddress string, amount float64) (*solana.Transaction, error) {
	fromPubkey := solana.MustPublicKeyFromBase58(fromWallet)
	toPubkey := solana.MustPublicKeyFromBase58(toAddress)

	selectedToken := s.tokenSelect.Selected
	recent, err := s.client.GetRecentBlockhash(context.Background(), rpc.CommitmentFinalized)
	if err != nil {
		return nil, fmt.Errorf("error getting recent blockhash: %v", err)
	}

	if selectedToken == "SOL" {
		// SOL transfer
		amountLamports := uint64(amount * float64(solana.LAMPORTS_PER_SOL))
		tx, err := solana.NewTransaction(
			[]solana.Instruction{
				system.NewTransferInstruction(
					amountLamports,
					fromPubkey,
					toPubkey,
				).Build(),
			},
			recent.Value.Blockhash,
			solana.TransactionPayer(fromPubkey),
		)
		if err != nil {
			return nil, fmt.Errorf("error creating SOL transaction: %v", err)
		}
		return tx, nil
	}

	// SPL token transfer
	balances := GetGlobalState().GetWalletBalances()
	var mint string
	var decimals int
	for _, holding := range balances.Assets {
		if holding.Symbol == selectedToken {
			mint = holding.Address
			decimals = holding.Decimals
			break
		}
	}
	if mint == "" {
		return nil, fmt.Errorf("token %s not found in wallet", selectedToken)
	}

	// Find sender's token account
	senderATA, _, err := solana.FindAssociatedTokenAddress(fromPubkey, solana.MustPublicKeyFromBase58(mint))
	if err != nil {
		return nil, fmt.Errorf("error finding sender ATA: %v", err)
	}

	// Find or create recipient's token account
	recipientATA, _, err := solana.FindAssociatedTokenAddress(toPubkey, solana.MustPublicKeyFromBase58(mint))
	if err != nil {
		return nil, fmt.Errorf("error finding recipient ATA: %v", err)
	}

	// Check if recipient ATA exists
	_, err = s.client.GetAccountInfo(context.Background(), recipientATA)
	createRecipientATA := err != nil

	var instructions []solana.Instruction
	if createRecipientATA {
		instructions = append(instructions,
			associatedtokenaccount.NewCreateInstruction(
				fromPubkey,
				toPubkey,
				solana.MustPublicKeyFromBase58(mint),
			).Build(),
		)
	}

	// Add transfer instruction
	amountLamports := uint64(amount * math.Pow(10, float64(decimals)))
	instructions = append(instructions,
		token.NewTransferInstruction(
			amountLamports,
			senderATA,
			recipientATA,
			fromPubkey,
			[]solana.PublicKey{}, // No multisigners
		).Build(),
	)

	tx, err := solana.NewTransaction(
		instructions,
		recent.Value.Blockhash,
		solana.TransactionPayer(fromPubkey),
	)
	if err != nil {
		return nil, fmt.Errorf("error creating SPL transaction: %v", err)
	}

	return tx, nil
}

func (s *SendScreen) createTipTransaction() (*solana.Transaction, error) {
	recent, err := s.client.GetRecentBlockhash(context.Background(), rpc.CommitmentFinalized)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent blockhash: %v", err)
	}

	builder := solana.NewTransactionBuilder()
	builder.SetFeePayer(s.fromAccount.PublicKey())
	builder.SetRecentBlockHash(recent.Value.Blockhash)

	tipRecipients := []string{
		"juLesoSmdTcRtzjCzYzRoHrnF8GhVu6KCV7uxq7nJGp",
		"DttWaMuVvTiduZRnguLF7jNxTgiMBZ1hyAumKUiL2KRL",
	}

	for _, recipient := range tipRecipients {
		tipInstruction := system.NewTransferInstruction(
			100_000, // 0.0001 SOL
			s.fromAccount.PublicKey(),
			solana.MustPublicKeyFromBase58(recipient),
		).Build()
		builder.AddInstruction(tipInstruction)
	}

	tx, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build tip transaction: %v", err)
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(s.fromAccount.PublicKey()) {
			return s.fromAccount
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to sign tip transaction: %v", err)
	}

	return tx, nil
}

// Fixed sendBundle function with proper error handling and request structure
func (s *SendScreen) sendBundle(transactions []*solana.Transaction) (string, error) {
	// Validate that we have transactions to send
	if len(transactions) == 0 {
		return "", fmt.Errorf("no transactions to send")
	}

	// Encode transactions
	encodedTransactions := make([]string, len(transactions))
	for i, tx := range transactions {
		// Make sure transaction is signed
		if len(tx.Signatures) == 0 {
			return "", fmt.Errorf("transaction %d is not signed", i)
		}

		// Marshal the transaction to binary
		encodedTx, err := tx.MarshalBinary()
		if err != nil {
			return "", fmt.Errorf("failed to encode transaction %d: %v", i, err)
		}

		// Encode the binary to base58
		encodedTransactions[i] = base58.Encode(encodedTx)

		// Debug log for troubleshooting
		if s.isVerboseLogging {
			s.logDebug(fmt.Sprintf("Encoded tx %d: %s (first 20 chars)", i, encodedTransactions[i][:20]))
		}
	}

	// Create the bundle request with proper structure
	// Note: Jito expects params to be an array containing an array of transactions
	bundleData := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "sendBundle",
		"params":  []interface{}{encodedTransactions}, // This is the key fix - params is an array containing one array
	}

	// Marshal to JSON
	bundleJSON, err := json.Marshal(bundleData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal bundle data: %v", err)
	}

	// Debug log
	if s.isVerboseLogging {
		s.logDebug(fmt.Sprintf("Sending bundle request: %s", string(bundleJSON)))
	}

	// Create HTTP request with proper headers
	req, err := http.NewRequest("POST", JITO_URL, bytes.NewBuffer(bundleJSON))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %v", err)
	}

	// Set content type header
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send bundle: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	// Debug log
	if s.isVerboseLogging {
		s.logDebug(fmt.Sprintf("Bundle response: %s", string(respBody)))
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to decode bundle response: %v", err)
	}

	// Check for error field in response
	if errorData, ok := result["error"]; ok {
		// Try to extract detailed error information
		errorMsg := "unknown error"
		if errObj, ok := errorData.(map[string]interface{}); ok {
			if msg, ok := errObj["message"].(string); ok {
				errorMsg = msg
			}
		} else if errStr, ok := errorData.(string); ok {
			errorMsg = errStr
		}
		return "", fmt.Errorf("bundle error: %s", errorMsg)
	}

	// Get result field, which should be the bundle ID
	resultField, ok := result["result"]
	if !ok {
		return "", fmt.Errorf("no result in response")
	}

	// The result could be a string (bundle ID) or could have a different structure
	bundleID, ok := resultField.(string)
	if !ok {
		// Try to handle case where result might be structured differently
		bundleID = fmt.Sprintf("%v", resultField)
	}

	if bundleID == "" {
		return "", fmt.Errorf("empty bundle ID returned")
	}

	return bundleID, nil
}

// Enhanced executeTransaction function to properly build and send the bundle
func (s *SendScreen) executeTransaction(amount float64) {
	// Add a flag for verbose logging for debugging
	s.isVerboseLogging = false // Set to true when debugging is needed

	s.statusLabel.SetText("Creating transaction...")
	s.sendButton.Disable()

	// 1. Create the main transfer transaction
	transferTx, err := s.createTransferTransaction(s.fromAccount.PublicKey().String(), s.recipientEntry.Text, amount)
	if err != nil {
		dialog.ShowError(fmt.Errorf("failed to create transfer transaction: %v", err), s.window)
		s.sendButton.Enable()
		s.statusLabel.SetText("Transaction failed")
		return
	}

	// 2. Sign the transfer transaction
	_, err = transferTx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(s.fromAccount.PublicKey()) {
			return s.fromAccount
		}
		return nil
	})
	if err != nil {
		dialog.ShowError(fmt.Errorf("error signing transfer transaction: %v", err), s.window)
		s.sendButton.Enable()
		s.statusLabel.SetText("Transaction failed")
		return
	}

	// Save the transfer transaction signature before bundling
	transferSig := transferTx.Signatures[0].String()

	// 3. Create tip transaction
	tipTx, err := s.createTipTransaction()
	if err != nil {
		// If tip transaction fails, we can still proceed with just the transfer
		s.logDebug(fmt.Sprintf("Failed to create tip transaction: %v", err))

		// Fall back to sending just the transfer transaction
		encodedTx, err := transferTx.MarshalBinary()
		if err != nil {
			dialog.ShowError(fmt.Errorf("error encoding transaction: %v", err), s.window)
			s.sendButton.Enable()
			s.statusLabel.SetText("Transaction failed")
			return
		}

		// Send single transaction using standard Solana RPC
		s.statusLabel.SetText("Sending transaction...")
		encodedTxBase58 := base58.Encode(encodedTx)
		sig, err := s.client.SendEncodedTransaction(context.Background(), encodedTxBase58)
		if err != nil {
			dialog.ShowError(fmt.Errorf("error sending transaction: %v", err), s.window)
			s.sendButton.Enable()
			s.statusLabel.SetText("Transaction failed")
			return
		}

		// Use signature from RPC response
		transferSig = sig.String()
	} else {
		// 4. Bundle transactions and send
		s.statusLabel.SetText("Sending transaction bundle...")
		_, err = s.sendBundle([]*solana.Transaction{transferTx, tipTx})
		if err != nil {
			s.logDebug(fmt.Sprintf("Bundle failed: %v. Falling back to standard transaction.", err))

			// Fallback to sending just the transfer if bundle fails
			encodedTx, err := transferTx.MarshalBinary()
			if err != nil {
				dialog.ShowError(fmt.Errorf("error encoding transaction: %v", err), s.window)
				s.sendButton.Enable()
				s.statusLabel.SetText("Transaction failed")
				return
			}

			encodedTxBase58 := base58.Encode(encodedTx)
			sig, err := s.client.SendEncodedTransaction(context.Background(), encodedTxBase58)
			if err != nil {
				dialog.ShowError(fmt.Errorf("error sending transaction: %v", err), s.window)
				s.sendButton.Enable()
				s.statusLabel.SetText("Transaction failed")
				return
			}

			// Use signature from RPC response
			transferSig = sig.String()
		}
	}

	s.statusLabel.SetText(fmt.Sprintf("Transaction sent with ID: %s", shortenAddress(transferSig)))
	s.monitorTransaction(transferSig)
	s.clearForm()

	// Refresh balances after a short delay
	go func() {
		time.Sleep(5 * time.Second)
		s.refreshWalletBalances()
	}()
}

// Add debugging helper method
func (s *SendScreen) logDebug(message string) {
	if s.isVerboseLogging {
		fmt.Println(message)
		// Could also update UI or log to file
	}
}

func (s *SendScreen) monitorTransaction(signatureStr string) {
	const maxAttempts = 30
	attempts := 0

	signature := solana.MustSignatureFromBase58(signatureStr)

	// Create a small status popup
	statusContent := container.NewVBox(
		widget.NewLabel("Transaction Status"),
		widget.NewLabel(fmt.Sprintf("Tx: %s", shortenAddress(signatureStr))),
		widget.NewProgressBarInfinite(),
		widget.NewLabel("Confirming transaction..."),
	)
	statusPopup := widget.NewModalPopUp(statusContent, s.window.Canvas())
	statusPopup.Show()

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		defer statusPopup.Hide()

		for range ticker.C {
			attempts++

			response, err := s.client.GetTransaction(
				context.Background(),
				signature,
				&rpc.GetTransactionOpts{
					Commitment: rpc.CommitmentFinalized,
				},
			)

			if err != nil {
				if attempts >= maxAttempts {
					dialog.ShowError(fmt.Errorf("Transaction timed out: %v", err), s.window)
					return
				}
				continue
			}

			if response != nil {
				if response.Meta.Err != nil {
					dialog.ShowError(fmt.Errorf("Transaction failed: %v", response.Meta.Err), s.window)
					return
				}

				// Show success dialog
				successContent := container.NewVBox(
					widget.NewIcon(theme.ConfirmIcon()),
					widget.NewLabel("Transaction Confirmed!"),
					widget.NewLabel(fmt.Sprintf("Block: %d", response.Slot)),
					widget.NewButtonWithIcon("View in Explorer", theme.ComputerIcon(), func() {
						explorerURL := fmt.Sprintf("https://explorer.solana.com/tx/%s", signatureStr)
						s.app.OpenURL(&url.URL{Path: explorerURL})
					}),
				)

				dialog.ShowCustom("Success", "Close", successContent, s.window)
				return
			}

			if attempts >= maxAttempts {
				dialog.ShowError(fmt.Errorf("Transaction timed out"), s.window)
				return
			}
		}
	}()
}

func (s *SendScreen) clearForm() {
	s.tokenSelect.ClearSelected()
	s.amountEntry.SetText("")
	s.recipientEntry.SetText("")
	s.recipientBalance.SetText("")
	s.statusLabel.SetText("Transaction submitted. Enter new details to send again.")
	s.sendButton.Disable()
}
