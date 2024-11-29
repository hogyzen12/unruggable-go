package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/validation"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/mr-tron/base58"
)

const (
	CALYPSO_ENDPOINT = "https://late-clean-snowflake.solana-mainnet.quiknode.pro/08c22e635ed0bae7fd982b2fbec90cad4086b169/"
	JITO_URL         = "https://mainnet.block-engine.jito.wtf/api/v1/bundles"
)

type SendScreen struct {
	container        *fyne.Container
	tokenSelect      *widget.Select
	amountEntry      *widget.Entry
	recipientEntry   *widget.Entry
	recipientBalance *widget.Label
	sendButton       *widget.Button
	statusLabel      *widget.Label
	window           fyne.Window
	client           *rpc.Client
	fromAccount      *solana.PrivateKey
	app              fyne.App
	selectedWalletID string // Track the selected wallet ID
}

type TransactionStatus struct {
	Signature     string
	Status        string
	Confirmations int
	Error         error
}

func NewSendScreen(window fyne.Window, app fyne.App) fyne.CanvasObject {
	s := &SendScreen{
		window:           window,
		app:              app,
		client:           rpc.New(CALYPSO_ENDPOINT),
		recipientBalance: widget.NewLabel(""),
		statusLabel:      widget.NewLabel(""),
	}

	// Get the globally selected wallet
	selectedWallet := GetGlobalState().GetSelectedWallet()
	if selectedWallet != "" {
		s.selectedWalletID = selectedWallet
		s.statusLabel.SetText(fmt.Sprintf("Using wallet: %s", shortenAddress(selectedWallet)))
	} else {
		s.statusLabel.SetText("No wallet selected. Please select a wallet from the Wallet tab.")
	}

	// Create the UI components
	title := canvas.NewText("Send Tokens", theme.ForegroundColor())
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter
	title.TextSize = 20

	// Token selection with custom styling
	s.tokenSelect = widget.NewSelect([]string{"SOL", "USDC", "JTO", "JUP", "JLP"}, s.onTokenSelected)
	s.tokenSelect.PlaceHolder = "Select token to send"

	// Amount entry with validation
	s.amountEntry = widget.NewEntry()
	s.amountEntry.SetPlaceHolder("Enter amount")
	s.amountEntry.Validator = validation.NewRegexp(`^[0-9]*\.?[0-9]*$`, "Must be a valid number")

	// Recipient address entry with validation
	s.recipientEntry = widget.NewEntry()
	s.recipientEntry.SetPlaceHolder("Enter recipient's Solana address")
	s.recipientEntry.OnChanged = s.validateAndFetchBalance
	s.recipientEntry.Validator = validation.NewRegexp(`^[1-9A-HJ-NP-Za-km-z]{43,44}$`, "Must be a valid Solana address")

	// Send button with styling
	s.sendButton = widget.NewButton("Send Transaction", s.handleSendTransaction)
	s.sendButton.Importance = widget.HighImportance
	s.sendButton.Disable()

	// Layout using containers with proper spacing
	form := container.NewVBox(
		container.NewPadded(title),
		widget.NewSeparator(),
		container.NewGridWithColumns(2,
			widget.NewLabel("Token:"),
			s.tokenSelect,
		),
		container.NewGridWithColumns(2,
			widget.NewLabel("Amount:"),
			s.amountEntry,
		),
		container.NewGridWithColumns(2,
			widget.NewLabel("Recipient:"),
			s.recipientEntry,
		),
		container.NewHBox(
			widget.NewIcon(theme.InfoIcon()),
			s.recipientBalance,
		),
		container.NewPadded(s.sendButton),
		container.NewHBox(
			widget.NewIcon(theme.InfoIcon()),
			s.statusLabel,
		),
	)

	// Add padding and scrolling for better mobile experience
	scroll := container.NewScroll(form)
	scroll.SetMinSize(fyne.NewSize(300, 400))

	s.container = container.NewPadded(scroll)
	return s.container
}

func (s *SendScreen) onTokenSelected(value string) {
	s.validateForm()
}

// Replace the validateAndFetchBalance function with this corrected version:
func (s *SendScreen) validateAndFetchBalance(address string) {
	s.recipientBalance.SetText("")
	s.validateForm()

	if address == "" {
		return
	}

	// Validate address format
	pubKey, err := solana.PublicKeyFromBase58(address)
	if err != nil {
		s.statusLabel.SetText("Invalid Solana address")
		return
	}

	// Fetch balance
	go func() {
		balance, err := s.client.GetBalance(
			context.Background(),
			pubKey,
			rpc.CommitmentFinalized,
		)
		if err != nil {
			s.recipientBalance.SetText("Error fetching balance")
			return
		}

		solBalance := float64(balance.Value) / float64(solana.LAMPORTS_PER_SOL)
		s.recipientBalance.SetText(fmt.Sprintf("Recipient Balance: %.9f SOL", solBalance))
	}()
}

// Add this helper function to validate Solana addresses
func isValidSolanaAddress(address string) bool {
	if len(address) != 44 && len(address) != 43 {
		return false
	}
	_, err := solana.PublicKeyFromBase58(address)
	return err == nil
}

// Update the validateForm function to use the new validation
func (s *SendScreen) validateForm() {
	isValid := s.tokenSelect.Selected != "" &&
		s.amountEntry.Text != "" &&
		s.recipientEntry.Text != "" &&
		s.amountEntry.Validate() == nil &&
		isValidSolanaAddress(s.recipientEntry.Text)

	if isValid {
		s.sendButton.Enable()
	} else {
		s.sendButton.Disable()
	}
}

func (s *SendScreen) handleSendTransaction() {
	if s.selectedWalletID == "" {
		dialog.ShowError(fmt.Errorf("no wallet selected - please select a wallet from the Wallet tab"), s.window)
		return
	}

	amount, err := strconv.ParseFloat(s.amountEntry.Text, 64)
	if err != nil {
		dialog.ShowError(fmt.Errorf("invalid amount"), s.window)
		return
	}

	// Show password dialog for decryption
	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.PlaceHolder = "Enter wallet password"

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
}

func (s *SendScreen) loadSelectedWallet(walletID string) {
	walletsDir := filepath.Join(s.app.Storage().RootURI().Path(), "wallets")
	filename := filepath.Join(walletsDir, walletID+".wallet")

	// Read the encrypted wallet file
	encryptedData, err := ioutil.ReadFile(filename)
	if err != nil {
		s.statusLabel.SetText(fmt.Sprintf("Error reading wallet file: %v", err))
		return
	}

	// Prompt for password
	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Enter wallet password")

	dialog.ShowCustomConfirm("Decrypt Wallet", "Unlock", "Cancel", passwordEntry, func(unlock bool) {
		if !unlock {
			return
		}

		decryptedKey, err := decrypt(string(encryptedData), passwordEntry.Text)
		if err != nil {
			dialog.ShowError(fmt.Errorf("Failed to decrypt wallet: %v", err), s.window)
			return
		}

		// Convert the decrypted private key to a Solana private key
		privateKey := solana.MustPrivateKeyFromBase58(string(decryptedKey))
		s.fromAccount = &privateKey

		s.statusLabel.SetText(fmt.Sprintf("Loaded wallet: %s", privateKey.PublicKey().String()))
	}, s.window)
}

// Add new method for creating transfer transaction
func (s *SendScreen) createTransferTransaction(fromWallet, toAddress string, amount float64) (*solana.Transaction, error) {
	fromPubkey := solana.MustPublicKeyFromBase58(fromWallet)
	toPubkey := solana.MustPublicKeyFromBase58(toAddress)

	// Convert amount to lamports
	amountLamports := uint64(amount * float64(solana.LAMPORTS_PER_SOL))

	recent, err := s.client.GetRecentBlockhash(context.Background(), rpc.CommitmentFinalized)
	if err != nil {
		return nil, fmt.Errorf("error getting recent blockhash: %v", err)
	}

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
		return nil, fmt.Errorf("error creating transaction: %v", err)
	}

	// Sign the transaction
	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(fromPubkey) {
			return s.fromAccount
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error signing transaction: %v", err)
	}

	return tx, nil
}

// Add createTipTransaction method (similar to CalypsoBot's implementation)
func (s *SendScreen) createTipTransaction() (*solana.Transaction, error) {
	recent, err := s.client.GetRecentBlockhash(context.Background(), rpc.CommitmentFinalized)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent blockhash: %v", err)
	}

	builder := solana.NewTransactionBuilder()
	builder.SetFeePayer(s.fromAccount.PublicKey())
	builder.SetRecentBlockHash(recent.Value.Blockhash)

	// Add tip transfers
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
		return nil, fmt.Errorf("failed to build transaction: %v", err)
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

// Replace the sendBundle method with this corrected version:
func (s *SendScreen) sendBundle(transactions []*solana.Transaction) (string, error) {
	encodedTransactions := make([]string, len(transactions))
	for i, tx := range transactions {
		encodedTx, err := tx.MarshalBinary()
		if err != nil {
			return "", fmt.Errorf("failed to encode transaction: %v", err)
		}
		encodedTransactions[i] = base58.Encode(encodedTx)
	}

	bundleData := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "sendBundle",
		"params":  []interface{}{encodedTransactions},
	}

	// Marshal the bundle data
	bundleJSON, err := json.Marshal(bundleData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal bundle data: %v", err)
	}

	resp, err := http.Post(JITO_URL, "application/json", bytes.NewBuffer(bundleJSON))
	if err != nil {
		return "", fmt.Errorf("failed to send bundle: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode bundle response: %v", err)
	}

	if errorData, ok := result["error"]; ok {
		return "", fmt.Errorf("bundle error: %v", errorData)
	}

	bundleID, ok := result["result"].(string)
	if !ok {
		return "", fmt.Errorf("invalid bundle response")
	}

	return bundleID, nil
}

func (s *SendScreen) listWalletFiles() ([]string, error) {
	walletsDir := filepath.Join(s.app.Storage().RootURI().Path(), "wallets")
	files, err := ioutil.ReadDir(walletsDir)
	if err != nil {
		return nil, err
	}

	var walletFiles []string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".wallet") {
			walletFiles = append(walletFiles, strings.TrimSuffix(file.Name(), ".wallet"))
		}
	}
	return walletFiles, nil
}

// New method to handle wallet decryption
func (s *SendScreen) decryptAndPrepareWallet(password string) error {
	walletsDir := filepath.Join(s.app.Storage().RootURI().Path(), "wallets")
	filename := filepath.Join(walletsDir, s.selectedWalletID+".wallet")

	encryptedData, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("error reading wallet file: %v", err)
	}

	decryptedKey, err := decrypt(string(encryptedData), password)
	if err != nil {
		return fmt.Errorf("failed to decrypt wallet: %v", err)
	}

	privateKey := solana.MustPrivateKeyFromBase58(string(decryptedKey))
	s.fromAccount = &privateKey
	return nil
}

func (s *SendScreen) executeTransaction(amount float64) {
	s.statusLabel.SetText("Creating transaction bundle...")
	s.sendButton.Disable()

	// Create the main transfer transaction
	transferTx, err := s.createTransferTransaction(s.fromAccount.PublicKey().String(), s.recipientEntry.Text, amount)
	if err != nil {
		dialog.ShowError(err, s.window)
		s.sendButton.Enable()
		s.statusLabel.SetText("Transaction failed")
		return
	}

	// Get the transfer transaction signature before bundling
	transferSig := transferTx.Signatures[0].String()

	// Create tip transaction
	tipTx, err := s.createTipTransaction()
	if err != nil {
		dialog.ShowError(err, s.window)
		s.sendButton.Enable()
		s.statusLabel.SetText("Transaction failed")
		return
	}

	// Bundle transactions
	bundleID, err := s.sendBundle([]*solana.Transaction{transferTx, tipTx})
	if err != nil {
		dialog.ShowError(err, s.window)
		s.sendButton.Enable()
		s.statusLabel.SetText("Transaction failed")
		return
	}

	s.statusLabel.SetText(fmt.Sprintf("Bundle sent: %s\nTracking transaction: %s",
		bundleID,
		transferSig,
	))

	// Start monitoring the main transaction
	s.monitorTransaction(transferSig)
	s.clearForm()
}

// Helper function to shorten addresses for display
func shortenAddress(address string) string {
	if len(address) <= 8 {
		return address
	}
	return fmt.Sprintf("%s...%s", address[:4], address[len(address)-4:])
}

func (s *SendScreen) monitorTransaction(signatureStr string) {
	const maxAttempts = 30
	attempts := 0

	// Parse signature into Solana signature type
	signature := solana.MustSignatureFromBase58(signatureStr)

	// Create status container
	statusContent := container.NewVBox()
	statusDialog := dialog.NewCustom("Transaction Status", "Close", statusContent, s.window)

	updateStatus := func(status string) {
		statusContent.Objects = []fyne.CanvasObject{
			widget.NewLabel(fmt.Sprintf("Transaction: %s", shortenAddress(signatureStr))),
			widget.NewLabel(fmt.Sprintf("Status: %s", status)),
			widget.NewProgressBarInfinite(),
		}
		statusDialog.Refresh()
	}

	updateStatus("Confirming...")
	statusDialog.Show()

	// Start monitoring in goroutine
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for range ticker.C {
			attempts++

			// Get transaction status
			response, err := s.client.GetTransaction(
				context.Background(),
				signature,
				&rpc.GetTransactionOpts{
					Commitment: rpc.CommitmentFinalized,
				},
			)

			if err != nil {
				if attempts >= maxAttempts {
					updateStatus(fmt.Sprintf("Failed to confirm: %v", err))
					return
				}
				continue
			}

			// Check if transaction is finalized
			if response != nil {
				if response.Meta.Err != nil {
					updateStatus(fmt.Sprintf("Transaction failed: %v", response.Meta.Err))
					return
				}

				// Transaction confirmed!
				statusContent.Objects = []fyne.CanvasObject{
					widget.NewIcon(theme.ConfirmIcon()),
					widget.NewLabel("Transaction Confirmed!"),
					widget.NewLabel(fmt.Sprintf("Signature: %s", shortenAddress(signatureStr))),
					widget.NewLabel(fmt.Sprintf("Block: %d", response.Slot)),
					widget.NewButtonWithIcon("View in Explorer", theme.ComputerIcon(), func() {
						explorerURL, _ := url.Parse(fmt.Sprintf("https://explorer.solana.com/tx/%s", signatureStr))
						s.app.OpenURL(explorerURL)
					}),
				}
				statusDialog.Refresh()
				return
			}

			if attempts >= maxAttempts {
				updateStatus("Timed out waiting for confirmation")
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
	s.sendButton.Enable()
}
