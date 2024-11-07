package ui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/hashicorp/vault/shamir"
)

const RPC_URL = "https://late-clean-snowflake.solana-mainnet.quiknode.pro/08c22e635ed0bae7fd982b2fbec90cad4086b169/"

type SigningScreen struct {
	container       *fyne.Container
	sessionIDEntry  *widget.Entry
	passwordEntry   *widget.Entry
	destAddrEntry   *widget.Entry
	amountEntry     *widget.Entry
	shareFileSelect *widget.Select
	statusLabel     *widget.Label
	logDisplay      *widget.Entry
	apiURL          string
	window          fyne.Window
	app             fyne.App
	clientID        string
}

func NewSigningScreen(window fyne.Window, app fyne.App) fyne.CanvasObject {
	s := &SigningScreen{
		sessionIDEntry: widget.NewEntry(),
		passwordEntry:  widget.NewPasswordEntry(),
		destAddrEntry:  widget.NewEntry(),
		amountEntry:    widget.NewEntry(),
		statusLabel:    widget.NewLabel(""),
		logDisplay:     widget.NewMultiLineEntry(),
		apiURL:         "https://sss-solana-deploy.fly.dev",
		window:         window,
		app:            app,
		clientID:       fmt.Sprintf("client_%d", time.Now().UnixNano()),
	}

	s.logDisplay.Disable()
	s.logDisplay.SetMinRowsVisible(9)
	s.passwordEntry.SetPlaceHolder("Enter password for share decryption")
	s.sessionIDEntry.SetPlaceHolder("Session ID (for participants)")
	s.destAddrEntry.SetPlaceHolder("Destination Solana address")
	s.amountEntry.SetPlaceHolder("Amount in SOL")

	// Get available share files
	shareFiles, err := s.listShareFiles()
	if err != nil {
		s.appendLog(fmt.Sprintf("Error listing share files: %v", err))
		shareFiles = []string{}
	}

	s.shareFileSelect = widget.NewSelect(shareFiles, func(value string) {
		s.appendLog(fmt.Sprintf("Selected share file: %s", value))
	})

	initiateButton := widget.NewButton("Initiate Signing", func() {
		go s.initiateSigningSession()
	})

	participateButton := widget.NewButton("Join Signing", func() {
		go s.participateInSigning()
	})

	// Layout
	form := container.NewGridWithColumns(2,
		widget.NewLabel("Share File:"), s.shareFileSelect,
		widget.NewLabel("Password:"), s.passwordEntry,
		widget.NewLabel("Destination:"), s.destAddrEntry,
		widget.NewLabel("Amount (SOL):"), s.amountEntry,
		widget.NewLabel("Session ID:"), s.sessionIDEntry,
	)

	buttons := container.NewHBox(initiateButton, participateButton)

	s.container = container.NewVBox(
		widget.NewLabel("Transaction Signing"),
		form,
		buttons,
		s.statusLabel,
		widget.NewLabel("Log:"),
		s.logDisplay,
	)

	return s.container
}

func (s *SigningScreen) initiateSigningSession() {
	if s.shareFileSelect.Selected == "" || s.passwordEntry.Text == "" {
		dialog.ShowError(fmt.Errorf("Please select a share file and enter password"), s.window)
		return
	}

	if s.destAddrEntry.Text == "" || s.amountEntry.Text == "" {
		dialog.ShowError(fmt.Errorf("Please enter destination address and amount"), s.window)
		return
	}

	// Load share file
	share, err := s.loadShareFile(s.shareFileSelect.Selected)
	if err != nil {
		s.showError("Failed to load share file", err)
		return
	}

	// Create signing session
	req := struct {
		PublicKey      string `json:"public_key"`
		PasswordHash   string `json:"password_hash"`
		RequiredShares int    `json:"required_shares"`
	}{
		PublicKey:      share.PublicKey,
		PasswordHash:   hashPassword([]byte(s.passwordEntry.Text)),
		RequiredShares: share.Threshold,
	}

	resp, err := http.Post(
		fmt.Sprintf("%s/create_signing_session", s.apiURL),
		"application/json",
		jsonEncode(req),
	)
	if err != nil {
		s.showError("Failed to create signing session", err)
		return
	}
	defer resp.Body.Close()

	var sessionResp struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		s.showError("Failed to decode session response", err)
		return
	}

	sessionID := sessionResp.SessionID
	s.sessionIDEntry.SetText(sessionID)
	s.appendLog(fmt.Sprintf("Created signing session: %s", sessionID))

	// Join own session
	if err := s.joinSigningSession(sessionID, share); err != nil {
		s.showError("Failed to join own session", err)
		return
	}

	// Submit share
	if err := s.submitShare(sessionID, share); err != nil {
		s.showError("Failed to submit share", err)
		return
	}

	s.appendLog(fmt.Sprintf("Waiting for %d additional participants...", share.Threshold-1))

	// Wait for other participants
	if err := s.waitForParticipants(sessionID, share.Threshold); err != nil {
		s.showError("Failed waiting for participants", err)
		return
	}

	// Create transaction
	amount := 0.0
	fmt.Sscanf(s.amountEntry.Text, "%f", &amount)

	tx, err := s.createSolanaTransaction(share.PublicKey, s.destAddrEntry.Text, amount)
	if err != nil {
		s.showError("Failed to create transaction", err)
		return
	}

	// Collect shares and reconstruct key
	shares, err := s.collectShares(sessionID, share)
	if err != nil {
		s.showError("Failed to collect shares", err)
		return
	}

	privateKey, err := s.reconstructPrivateKey(shares, share.PublicKey)
	if err != nil {
		s.showError("Failed to reconstruct private key", err)
		return
	}

	// Sign and broadcast
	signature, err := s.signAndBroadcastTransaction(privateKey, tx)
	if err != nil {
		s.showError("Failed to sign and broadcast transaction", err)
		return
	}

	s.appendLog(fmt.Sprintf("Transaction successful! Signature: %s", signature))
	dialog.ShowInformation("Success", "Transaction completed successfully", s.window)
}

func (s *SigningScreen) participateInSigning() {
	if s.shareFileSelect.Selected == "" || s.passwordEntry.Text == "" || s.sessionIDEntry.Text == "" {
		dialog.ShowError(fmt.Errorf("Please fill in all required fields"), s.window)
		return
	}

	share, err := s.loadShareFile(s.shareFileSelect.Selected)
	if err != nil {
		s.showError("Failed to load share file", err)
		return
	}

	sessionID := s.sessionIDEntry.Text

	// Join session
	if err := s.joinSigningSession(sessionID, share); err != nil {
		s.showError("Failed to join signing session", err)
		return
	}

	// Submit share
	if err := s.submitShare(sessionID, share); err != nil {
		s.showError("Failed to submit share", err)
		return
	}

	s.appendLog("Successfully participated in signing session")
	dialog.ShowInformation("Success", "Successfully submitted share for signing", s.window)
}

// Helper methods

func (s *SigningScreen) listShareFiles() ([]string, error) {
	sharesDir := filepath.Join(s.app.Storage().RootURI().Path(), "shares")
	files, err := ioutil.ReadDir(sharesDir)
	if err != nil {
		return nil, err
	}

	var shareFiles []string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".json") {
			shareFiles = append(shareFiles, file.Name())
		}
	}
	return shareFiles, nil
}

func (s *SigningScreen) loadShareFile(filename string) (*ShareFile, error) {
	sharesDir := filepath.Join(s.app.Storage().RootURI().Path(), "shares")
	data, err := ioutil.ReadFile(filepath.Join(sharesDir, filename))
	if err != nil {
		return nil, err
	}

	var share ShareFile
	if err := json.Unmarshal(data, &share); err != nil {
		return nil, err
	}

	return &share, nil
}

func (s *SigningScreen) joinSigningSession(sessionID string, share *ShareFile) error {
	joinReq := struct {
		SessionID    string `json:"session_id"`
		ClientID     string `json:"client_id"`
		PasswordHash string `json:"password_hash"`
	}{
		SessionID:    sessionID,
		ClientID:     s.clientID,
		PasswordHash: hashPassword([]byte(s.passwordEntry.Text)),
	}

	resp, err := http.Post(
		fmt.Sprintf("%s/join_signing_session", s.apiURL),
		"application/json",
		jsonEncode(joinReq),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("%s", string(bodyBytes))
	}

	s.appendLog("Successfully joined signing session")
	return nil
}

func (s *SigningScreen) submitShare(sessionID string, share *ShareFile) error {
	submitReq := struct {
		SessionID string         `json:"session_id"`
		ClientID  string         `json:"client_id"`
		Share     EncryptedShare `json:"share"`
	}{
		SessionID: sessionID,
		ClientID:  s.clientID,
		Share: EncryptedShare{
			Index:      share.ShareNumber,
			Data:       share.EncryptedShare,
			Nonce:      share.Nonce,
			PublicKey:  share.PublicKey,
			TotalCount: share.TotalShares,
			Threshold:  share.Threshold,
		},
	}

	resp, err := http.Post(
		fmt.Sprintf("%s/submit_share_for_signing", s.apiURL),
		"application/json",
		jsonEncode(submitReq),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("%s", string(bodyBytes))
	}

	s.appendLog("Successfully submitted share")
	return nil
}

func (s *SigningScreen) waitForParticipants(sessionID string, threshold int) error {
	for {
		resp, err := http.Get(fmt.Sprintf("%s/signing_session_status?session_id=%s", s.apiURL, sessionID))
		if err != nil {
			return err
		}

		var status struct {
			JoinedClients  int  `json:"joined_clients"`
			RequiredShares int  `json:"required_shares"`
			IsComplete     bool `json:"is_complete"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			resp.Body.Close()
			return err
		}
		resp.Body.Close()

		if status.JoinedClients == threshold {
			s.appendLog("All participants have joined!")
			break
		}

		s.appendLog(fmt.Sprintf("Waiting for participants (%d/%d)...",
			status.JoinedClients, threshold))
		time.Sleep(5 * time.Second)
	}
	return nil
}

func (s *SigningScreen) createSolanaTransaction(fromPubKey, toPubKey string, amount float64) (*solana.Transaction, error) {
	fromPubkey := solana.MustPublicKeyFromBase58(fromPubKey)
	toPubkey := solana.MustPublicKeyFromBase58(toPubKey)
	amountLamports := uint64(amount * float64(solana.LAMPORTS_PER_SOL))

	client := rpc.New(RPC_URL)
	ctx := context.Background()

	recent, err := client.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return nil, err
	}

	instruction := system.NewTransferInstruction(
		amountLamports,
		fromPubkey,
		toPubkey,
	).Build()

	return solana.NewTransaction(
		[]solana.Instruction{instruction},
		recent.Value.Blockhash,
		solana.TransactionPayer(fromPubkey),
	)
}

func (s *SigningScreen) collectShares(sessionID string, myShare *ShareFile) ([][]byte, error) {
	shares := make([][]byte, 0, myShare.Threshold)

	// First, decrypt our own share
	s.appendLog("Decrypting own share...")
	decryptedShare, err := decryptShare(myShare.EncryptedShare, myShare.Nonce, []byte(s.passwordEntry.Text))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt own share: %v", err)
	}
	shares = append(shares, decryptedShare)

	// Get other shares
	s.appendLog("Fetching shares from other participants...")
	resp, err := http.Get(fmt.Sprintf("%s/get_signing_shares?session_id=%s&client_id=%s",
		s.apiURL, sessionID, s.clientID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var response struct {
		Success bool        `json:"success"`
		Shares  []ShareFile `json:"shares"`
	}
	// Continuing the collectShares method...
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode shares response: %v", err)
	}

	for i, share := range response.Shares {
		if share.ShareNumber == myShare.ShareNumber {
			continue // Skip our own share
		}
		s.appendLog(fmt.Sprintf("Decrypting share %d...", share.ShareNumber))
		decryptedShare, err := decryptShare(share.EncryptedShare, share.Nonce, []byte(s.passwordEntry.Text))
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt share %d: %v", i+1, err)
		}
		shares = append(shares, decryptedShare)
	}

	if len(shares) < myShare.Threshold {
		return nil, fmt.Errorf("insufficient shares: got %d, need %d", len(shares), myShare.Threshold)
	}

	return shares, nil
}

func (s *SigningScreen) reconstructPrivateKey(shares [][]byte, publicKey string) (solana.PrivateKey, error) {
	secret, err := shamir.Combine(shares)
	if err != nil {
		return nil, fmt.Errorf("failed to combine shares: %v", err)
	}

	pubkey := solana.MustPublicKeyFromBase58(publicKey)
	privateKeyBytes := append(secret, pubkey[:]...)
	privateKey := solana.PrivateKey(privateKeyBytes)

	if !privateKey.PublicKey().Equals(pubkey) {
		return nil, fmt.Errorf("reconstructed key verification failed")
	}

	return privateKey, nil
}

func (s *SigningScreen) signAndBroadcastTransaction(privateKey solana.PrivateKey, tx *solana.Transaction) (string, error) {
	tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(privateKey.PublicKey()) {
			return &privateKey
		}
		return nil
	})

	rawTx, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %v", err)
	}

	encodedTx := base64.StdEncoding.EncodeToString(rawTx)
	client := rpc.New(RPC_URL)
	ctx := context.Background()

	sig, err := client.SendEncodedTransaction(ctx, encodedTx)
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %v", err)
	}

	return sig.String(), nil
}

func (s *SigningScreen) appendLog(message string) {
	s.logDisplay.SetText(s.logDisplay.Text + message + "\n")
	s.logDisplay.CursorRow = len(strings.Split(s.logDisplay.Text, "\n")) - 1
	s.logDisplay.Refresh()
}

func (s *SigningScreen) showError(message string, err error) {
	s.appendLog(fmt.Sprintf("Error: %s - %v", message, err))
	dialog.ShowError(fmt.Errorf("%s: %v", message, err), s.window)
}

// End of signing_screen.go
