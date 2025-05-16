package ui

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/mr-tron/base58"
)

// TransactionInspector represents the transaction inspection screen
type TransactionInspector struct {
	window              fyne.Window
	app                 fyne.App
	container           *fyne.Container
	txInput             *widget.Entry
	resultOutput        *widget.Entry
	decodeButton        *widget.Button
	fetchButton         *widget.Button
	statusLabel         *widget.Label
	formatSelector      *widget.RadioGroup
	client              *rpc.Client
	showSignaturesBtn   *widget.Button
	showAccountsBtn     *widget.Button
	showInstructionsBtn *widget.Button
	viewMode            string
	currentTx           *solana.Transaction
}

// NewTransactionInspectorScreen creates a new transaction inspection screen
func NewTransactionInspectorScreen(window fyne.Window, app fyne.App) fyne.CanvasObject {
	inspector := &TransactionInspector{
		window:   window,
		app:      app,
		client:   rpc.New(CALYPSO_ENDPOINT),
		viewMode: "full", // Default view mode
	}

	inspector.txInput = widget.NewMultiLineEntry()
	inspector.txInput.SetPlaceHolder("Paste transaction signature or encoded transaction (base58 or base64)")
	inspector.txInput.SetMinRowsVisible(3)

	inspector.resultOutput = widget.NewMultiLineEntry()
	inspector.resultOutput.SetPlaceHolder("Transaction details will appear here")
	inspector.resultOutput.SetMinRowsVisible(15)
	inspector.resultOutput.Disable() // Read-only

	inspector.formatSelector = widget.NewRadioGroup(
		[]string{"Auto-detect", "Base58", "Base64", "Signature"},
		func(selected string) {
			// Clear any previous results when changing format
			inspector.resultOutput.SetText("")
		},
	)
	inspector.formatSelector.SetSelected("Auto-detect")
	inspector.formatSelector.Horizontal = true

	inspector.decodeButton = widget.NewButtonWithIcon("Decode Transaction", theme.SearchIcon(), func() {
		inspector.decodeTransaction()
	})

	inspector.fetchButton = widget.NewButtonWithIcon("Fetch by Signature", theme.DownloadIcon(), func() {
		inspector.fetchBySignature()
	})

	// Button to copy transaction to clipboard
	copyButton := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		window.Clipboard().SetContent(inspector.resultOutput.Text)
		inspector.statusLabel.SetText("Output copied to clipboard")
	})
	copyButton.Importance = widget.LowImportance

	// View mode buttons
	inspector.showSignaturesBtn = widget.NewButton("Signatures", func() {
		inspector.viewMode = "signatures"
		inspector.updateOutput()
	})

	inspector.showAccountsBtn = widget.NewButton("Accounts", func() {
		inspector.viewMode = "accounts"
		inspector.updateOutput()
	})

	inspector.showInstructionsBtn = widget.NewButton("Instructions", func() {
		inspector.viewMode = "instructions"
		inspector.updateOutput()
	})

	fullViewBtn := widget.NewButton("Full View", func() {
		inspector.viewMode = "full"
		inspector.updateOutput()
	})

	// Buttons initially disabled until we have a transaction
	inspector.showSignaturesBtn.Disable()
	inspector.showAccountsBtn.Disable()
	inspector.showInstructionsBtn.Disable()
	fullViewBtn.Disable()

	// Status label for feedback
	inspector.statusLabel = widget.NewLabel("")

	// Layout the UI elements
	formatBox := container.NewVBox(
		widget.NewLabelWithStyle("Format:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		inspector.formatSelector,
	)

	actionButtons := container.NewGridWithColumns(2,
		inspector.decodeButton,
		inspector.fetchButton,
	)

	viewButtons := container.NewGridWithColumns(4,
		fullViewBtn,
		inspector.showSignaturesBtn,
		inspector.showAccountsBtn,
		inspector.showInstructionsBtn,
	)

	outputHeader := container.NewBorder(
		nil, nil,
		widget.NewLabelWithStyle("Transaction Details:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		copyButton,
	)

	content := container.NewVBox(
		widget.NewLabelWithStyle("Transaction Inspector", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		formatBox,
		inspector.txInput,
		actionButtons,
		widget.NewSeparator(),
		outputHeader,
		viewButtons,
		inspector.resultOutput,
		inspector.statusLabel,
	)

	inspector.container = container.NewPadded(
		container.NewVScroll(content),
	)

	return inspector.container
}

// decodeTransaction attempts to decode the transaction in the input field
func (t *TransactionInspector) decodeTransaction() {
	inputText := strings.TrimSpace(t.txInput.Text)
	if inputText == "" {
		t.statusLabel.SetText("Please enter a transaction")
		return
	}

	format := t.formatSelector.Selected
	if format == "Signature" {
		t.fetchBySignature()
		return
	}

	var tx *solana.Transaction
	var err error

	t.statusLabel.SetText("Decoding transaction...")

	// Try auto-detection or use the selected format
	if format == "Auto-detect" {
		tx, err = t.tryDecodeAutodetect(inputText)
	} else if format == "Base58" {
		tx, err = t.decodeBase58(inputText)
	} else if format == "Base64" {
		tx, err = t.decodeBase64(inputText)
	}

	if err != nil {
		t.statusLabel.SetText(fmt.Sprintf("Error: %v", err))
		return
	}

	if tx != nil {
		t.currentTx = tx
		t.updateOutput()
		t.enableViewButtons()
		t.statusLabel.SetText("Transaction decoded successfully")
	}
}

// tryDecodeAutodetect attempts to decode the transaction in multiple formats
func (t *TransactionInspector) tryDecodeAutodetect(input string) (*solana.Transaction, error) {
	// Try Base58 first (more common for Solana)
	tx, err := t.decodeBase58(input)
	if err == nil {
		return tx, nil
	}

	// Try Base64
	tx, err = t.decodeBase64(input)
	if err == nil {
		return tx, nil
	}

	// If both failed, but the input is a likely signature, try fetching
	if len(input) >= 43 && len(input) <= 88 {
		t.txInput.SetText(input)
		t.formatSelector.SetSelected("Signature")
		go t.fetchBySignature()
		return nil, fmt.Errorf("not a valid encoded transaction, trying as signature")
	}

	return nil, fmt.Errorf("invalid transaction format or data")
}

// decodeBase58 decodes a base58 encoded transaction
func (t *TransactionInspector) decodeBase58(encoded string) (*solana.Transaction, error) {
	data, err := base58.Decode(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base58: %v", err)
	}

	return t.decodeBinary(data)
}

// decodeBase64 decodes a base64 encoded transaction
func (t *TransactionInspector) decodeBase64(encoded string) (*solana.Transaction, error) {
	// Remove any padding that might have been added
	encoded = strings.TrimSpace(encoded)

	// Try standard base64
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		// Try URL-safe variant
		data, err = base64.URLEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64: %v", err)
		}
	}

	return t.decodeBinary(data)
}

// decodeBinary decodes binary transaction data
func (t *TransactionInspector) decodeBinary(data []byte) (*solana.Transaction, error) {
	tx, err := solana.TransactionFromDecoder(bin.NewBinDecoder(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode transaction: %v", err)
	}
	return tx, nil
}

// fetchBySignature fetches a transaction by its signature
func (t *TransactionInspector) fetchBySignature() {
	signature := strings.TrimSpace(t.txInput.Text)
	if signature == "" {
		t.statusLabel.SetText("Please enter a transaction signature")
		return
	}

	t.statusLabel.SetText("Fetching transaction...")

	// Validate the signature format
	_, err := solana.SignatureFromBase58(signature)
	if err != nil {
		t.statusLabel.SetText(fmt.Sprintf("Invalid signature format: %v", err))
		return
	}

	// Show a progress dialog
	progress := dialog.NewProgressInfinite("Fetching Transaction", "Retrieving from Solana network...", t.window)
	progress.Show()

	go func() {
		// Fetch transaction with binary encoding
		txSig := solana.MustSignatureFromBase58(signature)
		out, err := t.client.GetTransaction(
			context.Background(),
			txSig,
			&rpc.GetTransactionOpts{
				Encoding: solana.EncodingBase64,
			},
		)

		// We need to update UI on the main thread
		// This is done by using a goroutine to update UI elements
		// Similar to how it's done in your other screens

		// Using common go patterns for thread-safety
		var tx *solana.Transaction
		var decodeErr error

		if err == nil && out != nil && out.Transaction != nil {
			// Decode the transaction binary
			txData := out.Transaction.GetBinary()
			if txData != nil && len(txData) > 0 {
				tx, decodeErr = solana.TransactionFromDecoder(bin.NewBinDecoder(txData))
			}
		}

		// Update the UI from the main thread - we simply defer operations
		// until we return to the main thread - just like in conditional_bot.go
		t.window.Canvas().Refresh(t.container)

		// Make sure to hide progress dialog regardless of result
		progress.Hide()

		if err != nil {
			t.statusLabel.SetText(fmt.Sprintf("Error fetching transaction: %v", err))
			return
		}

		if out == nil || out.Transaction == nil {
			t.statusLabel.SetText("Transaction not found or returned empty")
			return
		}

		// Check if we had decoded the transaction
		if decodeErr != nil {
			t.statusLabel.SetText(fmt.Sprintf("Error decoding transaction: %v", decodeErr))
			return
		}

		if tx == nil {
			t.statusLabel.SetText("Transaction binary data not available")
			return
		}

		// Set the transaction and update UI
		t.currentTx = tx
		t.updateOutput()
		t.enableViewButtons()
		t.statusLabel.SetText("Transaction loaded successfully")
	}()
}

// updateOutput updates the output text based on the view mode
func (t *TransactionInspector) updateOutput() {
	if t.currentTx == nil {
		return
	}

	var output string

	switch t.viewMode {
	case "signatures":
		output = t.formatSignatures()
	case "accounts":
		output = t.formatAccounts()
	case "instructions":
		output = t.formatInstructions()
	default: // full view
		output = t.formatFullTransaction()
	}

	t.resultOutput.SetText(output)
}

// formatSignatures returns a formatted string of transaction signatures
func (t *TransactionInspector) formatSignatures() string {
	var buffer bytes.Buffer

	buffer.WriteString("Transaction Signatures:\n")
	buffer.WriteString("====================\n\n")

	for i, sig := range t.currentTx.Signatures {
		buffer.WriteString(fmt.Sprintf("%d. %s\n", i+1, sig.String()))
	}

	return buffer.String()
}

// formatAccounts returns a formatted string of transaction accounts
func (t *TransactionInspector) formatAccounts() string {
	var buffer bytes.Buffer

	buffer.WriteString("Transaction Accounts:\n")
	buffer.WriteString("====================\n\n")

	// Fee payer
	buffer.WriteString(fmt.Sprintf("Fee Payer: %s\n\n", t.currentTx.Message.AccountKeys[0].String()))

	// Account list with metadata
	buffer.WriteString("All Accounts:\n")
	for i, acc := range t.currentTx.Message.AccountKeys {
		// Check if account is a signer
		isSigner := i < int(t.currentTx.Message.Header.NumRequiredSignatures)

		// Check if account is writable
		// For simplicity, we'll assume the first NumRequiredSignatures accounts are writable
		// This is a simplification - in reality writable accounts are determined by
		// NumRequiredSignatures and NumReadonlySigners
		isWritable := i < int(t.currentTx.Message.Header.NumRequiredSignatures-
			t.currentTx.Message.Header.NumReadonlySignedAccounts)

		attributes := []string{}
		if isSigner {
			attributes = append(attributes, "Signer")
		}
		if isWritable {
			attributes = append(attributes, "Writable")
		}

		attrStr := ""
		if len(attributes) > 0 {
			attrStr = " (" + strings.Join(attributes, ", ") + ")"
		}

		buffer.WriteString(fmt.Sprintf("%d. %s%s\n", i+1, acc.String(), attrStr))
	}

	return buffer.String()
}

// formatInstructions returns a formatted string of transaction instructions
func (t *TransactionInspector) formatInstructions() string {
	var buffer bytes.Buffer

	buffer.WriteString("Transaction Instructions:\n")
	buffer.WriteString("=======================\n\n")

	for i, inst := range t.currentTx.Message.Instructions {
		// Find program ID
		programIdx := inst.ProgramIDIndex
		programID := t.currentTx.Message.AccountKeys[programIdx]

		buffer.WriteString(fmt.Sprintf("Instruction %d:\n", i+1))
		buffer.WriteString(fmt.Sprintf("Program: %s\n", programID.String()))

		// List accounts used by this instruction
		buffer.WriteString("Accounts:\n")
		for j, accIdx := range inst.Accounts {
			buffer.WriteString(fmt.Sprintf("  %d. %s\n", j+1, t.currentTx.Message.AccountKeys[accIdx].String()))
		}

		// Show data (hex encoded)
		buffer.WriteString(fmt.Sprintf("Data: %x\n", inst.Data))
		buffer.WriteString("\n")
	}

	return buffer.String()
}

// formatFullTransaction returns a full formatted transaction
func (t *TransactionInspector) formatFullTransaction() string {
	var buffer bytes.Buffer

	// Helper to add a separator line
	addSeparator := func() {
		buffer.WriteString("\n----------------------------------------------------\n\n")
	}

	// Overview
	buffer.WriteString("TRANSACTION OVERVIEW\n")
	buffer.WriteString("====================\n\n")
	buffer.WriteString(fmt.Sprintf("Signatures: %d\n", len(t.currentTx.Signatures)))
	buffer.WriteString(fmt.Sprintf("Accounts: %d\n", len(t.currentTx.Message.AccountKeys)))
	buffer.WriteString(fmt.Sprintf("Instructions: %d\n", len(t.currentTx.Message.Instructions)))
	buffer.WriteString(fmt.Sprintf("Recent Blockhash: %s\n", t.currentTx.Message.RecentBlockhash))

	// Signatures
	addSeparator()
	buffer.WriteString("SIGNATURES\n")
	buffer.WriteString("==========\n\n")

	for i, sig := range t.currentTx.Signatures {
		buffer.WriteString(fmt.Sprintf("%d. %s\n", i+1, sig.String()))
	}

	// Accounts
	addSeparator()
	buffer.WriteString("ACCOUNTS\n")
	buffer.WriteString("========\n\n")

	for i, acc := range t.currentTx.Message.AccountKeys {
		buffer.WriteString(fmt.Sprintf("%d. %s\n", i+1, acc.String()))
	}

	// Instructions
	addSeparator()
	buffer.WriteString("INSTRUCTIONS\n")
	buffer.WriteString("============\n\n")

	for i, inst := range t.currentTx.Message.Instructions {
		programIdx := inst.ProgramIDIndex
		programID := t.currentTx.Message.AccountKeys[programIdx]

		buffer.WriteString(fmt.Sprintf("Instruction %d:\n", i+1))
		buffer.WriteString(fmt.Sprintf("Program: %s\n", programID.String()))

		buffer.WriteString("Accounts:\n")
		for j, accIdx := range inst.Accounts {
			buffer.WriteString(fmt.Sprintf("  %d. %s\n", j+1, t.currentTx.Message.AccountKeys[accIdx].String()))
		}

		buffer.WriteString(fmt.Sprintf("Data: %x\n\n", inst.Data))
	}

	return buffer.String()
}

// enableViewButtons enables the view mode buttons when a transaction is loaded
func (t *TransactionInspector) enableViewButtons() {
	t.showSignaturesBtn.Enable()
	t.showAccountsBtn.Enable()
	t.showInstructionsBtn.Enable()

	// Find and enable the full view button
	for _, obj := range t.container.Objects[0].(*container.Scroll).Content.(*fyne.Container).Objects {
		if vBox, ok := obj.(*fyne.Container); ok {
			for _, child := range vBox.Objects {
				if grid, ok := child.(*fyne.Container); ok {
					if len(grid.Objects) > 0 {
						if btn, ok := grid.Objects[0].(*widget.Button); ok {
							if btn.Text == "Full View" {
								btn.Enable()
								break
							}
						}
					}
				}
			}
		}
	}
}
