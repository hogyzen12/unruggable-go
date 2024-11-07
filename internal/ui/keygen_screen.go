package ui

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/gagliardetto/solana-go"
	"github.com/hashicorp/vault/shamir"
)

type KeygenScreen struct {
	container       *fyne.Container
	sessionIDEntry  *widget.Entry
	tEntry          *widget.Entry
	nEntry          *widget.Entry
	passwordEntry   *widget.Entry
	statusLabel     *widget.Label
	logDisplay      *widget.Entry
	apiURL          string
	window          fyne.Window
	app             fyne.App
	clientID        string
	shareFileSelect *widget.Select
}

type EncryptedShare struct {
	Index      int    `json:"index"`
	Data       string `json:"data"`
	Nonce      string `json:"nonce"`
	PublicKey  string `json:"public_key"`
	TotalCount int    `json:"total_count"`
	Threshold  int    `json:"threshold"`
}

type ShareFile struct {
	ShareNumber    int       `json:"share_number"`
	PublicKey      string    `json:"public_key"`
	EncryptedShare string    `json:"encrypted_share"`
	Nonce          string    `json:"nonce"`
	TotalShares    int       `json:"total_shares"`
	Threshold      int       `json:"threshold"`
	CreatedAt      time.Time `json:"created_at"`
	SessionID      string    `json:"session_id"`
}

func NewKeygenScreen(window fyne.Window, app fyne.App) fyne.CanvasObject {
	k := &KeygenScreen{
		sessionIDEntry: widget.NewEntry(),
		tEntry:         widget.NewEntry(),
		nEntry:         widget.NewEntry(),
		passwordEntry:  widget.NewPasswordEntry(),
		statusLabel:    widget.NewLabel(""),
		logDisplay:     widget.NewMultiLineEntry(),
		apiURL:         "https://sss-solana-deploy.fly.dev",
		window:         window,
		app:            app,
		clientID:       fmt.Sprintf("client_%d", time.Now().UnixNano()),
	}

	k.logDisplay.Disable()
	k.logDisplay.SetMinRowsVisible(9)
	k.passwordEntry.SetPlaceHolder("Enter password for share encryption")

	// Get available share files
	shareFiles, err := k.listShareFiles()
	if err != nil {
		k.appendLog(fmt.Sprintf("Error listing share files: %v", err))
		shareFiles = []string{}
	}

	k.shareFileSelect = widget.NewSelect(shareFiles, func(value string) {
		k.appendLog(fmt.Sprintf("Selected share file: %s", value))
	})

	// Layout for threshold and total parties
	thresholdPartyForm := container.NewGridWithColumns(4,
		widget.NewLabel("Threshold (t):"),
		k.tEntry,
		widget.NewLabel("Total Parties (n):"),
		k.nEntry,
	)

	initButton := widget.NewButton("Initialize Session", func() {
		go k.initiateKeygenSession()
	})

	sessionIDForm := container.NewBorder(nil, nil, widget.NewLabel("Session ID:"), nil, k.sessionIDEntry)
	joinButton := widget.NewButton("Join Session", func() {
		go k.joinKeygenSession()
	})

	k.container = container.NewVBox(
		widget.NewLabel("Keygen Setup"),
		thresholdPartyForm,
		k.passwordEntry,
		initButton,
		sessionIDForm,
		joinButton,
		k.statusLabel,
		widget.NewLabel("Share Files:"),
		k.shareFileSelect,
		widget.NewLabel("Log:"),
		k.logDisplay,
	)

	return k.container
}

// Add new method for listing share files
func (k *KeygenScreen) listShareFiles() ([]string, error) {
	sharesDir := k.getSharesDirectory()
	files, err := ioutil.ReadDir(sharesDir)
	if err != nil {
		return nil, err
	}

	var shareFiles []string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".json") {
			// Read file to check if it's a keygen share
			data, err := ioutil.ReadFile(filepath.Join(sharesDir, file.Name()))
			if err != nil {
				continue
			}

			var shareFile ShareFile
			if err := json.Unmarshal(data, &shareFile); err != nil {
				continue
			}

			// Only include files that match our ShareFile structure
			if shareFile.ShareNumber > 0 && shareFile.PublicKey != "" {
				shareFiles = append(shareFiles, file.Name())
			}
		}
	}
	return shareFiles, nil
}

// Add method to refresh share files list
func (k *KeygenScreen) refreshSharesList() {
	shareFiles, err := k.listShareFiles()
	if err != nil {
		k.appendLog(fmt.Sprintf("Error refreshing shares list: %v", err))
		return
	}

	k.shareFileSelect.Options = shareFiles
	k.shareFileSelect.Refresh()
}

func (k *KeygenScreen) initiateKeygenSession() {
	t := k.tEntry.Text
	n := k.nEntry.Text
	password := k.passwordEntry.Text

	if t == "" || n == "" || password == "" {
		dialog.ShowError(fmt.Errorf("Please fill in all fields"), k.window)
		return
	}

	threshold, numShares := 0, 0
	fmt.Sscanf(t, "%d", &threshold)
	fmt.Sscanf(n, "%d", &numShares)

	if threshold < 2 || threshold > numShares {
		dialog.ShowError(fmt.Errorf("Invalid threshold or total shares"), k.window)
		return
	}

	k.appendLog(fmt.Sprintf("Initializing session with t=%d and n=%d", threshold, numShares))

	// Create session
	createReq := struct {
		RequiredClients int    `json:"required_clients"`
		PasswordHash    string `json:"password_hash"`
	}{
		RequiredClients: numShares,
		PasswordHash:    hashPassword([]byte(password)),
	}

	// Debug log the request
	reqBytes, _ := json.MarshalIndent(createReq, "", "  ")
	k.appendLog(fmt.Sprintf("Sending create session request:\n%s", string(reqBytes)))

	resp, err := http.Post(
		fmt.Sprintf("%s/create_session", k.apiURL),
		"application/json",
		jsonEncode(createReq),
	)
	if err != nil {
		k.showError("Failed to create session", err)
		return
	}
	defer resp.Body.Close()

	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	k.appendLog(fmt.Sprintf("Create session response:\n%s", string(bodyBytes)))

	var sessionResp struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(bodyBytes, &sessionResp); err != nil {
		k.showError("Failed to decode session response", err)
		return
	}

	sessionID := sessionResp.SessionID
	k.sessionIDEntry.SetText(sessionID)
	k.appendLog(fmt.Sprintf("Created session: %s", sessionID))
	k.appendLog(fmt.Sprintf("Waiting for %d additional clients to join...", numShares-1))

	// Join own session
	if err := k.joinSession(sessionID, password); err != nil {
		k.showError("Failed to join own session", err)
		return
	}

	// Wait for all clients to join
	k.appendLog("Waiting for all participants to join...")
	if err := k.waitForParticipants(sessionID, numShares); err != nil {
		k.showError("Failed waiting for participants", err)
		return
	}

	k.appendLog("All participants have joined! Generating keypair...")

	// Generate and split keypair
	wallet := solana.NewWallet()
	privateKey := wallet.PrivateKey
	publicKey := privateKey.PublicKey().String()

	k.appendLog(fmt.Sprintf("Generated keypair with public key: %s", publicKey))

	// Split private key
	shares, err := k.splitPrivateKey(privateKey, numShares, threshold)
	if err != nil {
		k.showError("Failed to split private key", err)
		return
	}

	// Encrypt shares
	encryptedShares := make([]EncryptedShare, len(shares))
	for i, share := range shares {
		encryptedData, nonce, err := k.encryptShare(share, []byte(password))
		if err != nil {
			k.showError(fmt.Sprintf("Failed to encrypt share %d", i+1), err)
			return
		}

		encryptedShares[i] = EncryptedShare{
			Index:      i + 1,
			Data:       encryptedData,
			Nonce:      nonce,
			PublicKey:  publicKey,
			TotalCount: numShares,
			Threshold:  threshold,
		}

		// Save initiator's own share immediately
		if i == 0 {
			if err := k.saveShareToFile(&encryptedShares[i], sessionID); err != nil {
				k.showError("Failed to save initiator share", err)
				return
			}
			k.appendLog(fmt.Sprintf("Saved initiator's share %d", i+1))
		}
	}

	// Submit shares
	k.appendLog("Submitting encrypted shares to server...")

	shareData := struct {
		SessionID string           `json:"session_id"`
		ClientID  string           `json:"client_id"`
		Shares    []EncryptedShare `json:"shares"`
		PublicKey string           `json:"public_key"`
	}{
		SessionID: sessionID,
		ClientID:  k.clientID,
		Shares:    encryptedShares,
		PublicKey: publicKey,
	}

	// Debug log the share submission
	shareDataBytes, _ := json.MarshalIndent(shareData, "", "  ")
	k.appendLog(fmt.Sprintf("Share submission payload:\n%s", string(shareDataBytes)))

	resp, err = http.Post(
		fmt.Sprintf("%s/submit_shares", k.apiURL),
		"application/json",
		jsonEncode(shareData),
	)
	if err != nil {
		k.showError("Failed to submit shares", err)
		return
	}
	defer resp.Body.Close()

	respBody, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		k.showError("Failed to submit shares", fmt.Errorf("server responded with status %d: %s",
			resp.StatusCode, string(respBody)))
		return
	}

	k.appendLog("Successfully submitted encrypted shares")
	k.appendLog(fmt.Sprintf("Response: %s", string(respBody)))
	k.refreshSharesList()

	dialog.ShowInformation("Success",
		fmt.Sprintf("Successfully completed keygen. Your share has been saved.\nSession ID: %s", sessionID),
		k.window)
}

// Add new helper method for waiting for participants
func (k *KeygenScreen) waitForParticipants(sessionID string, required int) error {
	for {
		resp, err := http.Get(fmt.Sprintf("%s/session_status?session_id=%s", k.apiURL, sessionID))
		if err != nil {
			return fmt.Errorf("failed to get session status: %v", err)
		}

		var status struct {
			JoinedClients   int  `json:"joined_clients"`
			RequiredClients int  `json:"required_clients"`
			IsComplete      bool `json:"is_complete"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			resp.Body.Close()
			return fmt.Errorf("failed to decode status response: %v", err)
		}
		resp.Body.Close()

		k.appendLog(fmt.Sprintf("Current participants: %d/%d", status.JoinedClients, required))

		if status.JoinedClients >= required {
			k.appendLog("All participants have joined!")
			return nil
		}

		time.Sleep(2 * time.Second)
	}
}

func (k *KeygenScreen) joinKeygenSession() {
	sessionID := k.sessionIDEntry.Text
	password := k.passwordEntry.Text

	if sessionID == "" || password == "" {
		dialog.ShowError(fmt.Errorf("Please enter session ID and password"), k.window)
		return
	}

	if err := k.joinSession(sessionID, password); err != nil {
		k.showError("Failed to join session", err)
		return
	}

	k.appendLog("Successfully joined session")

	// Poll for share
	for {
		share, err := k.getShare(sessionID)
		if err != nil {
			k.appendLog(fmt.Sprintf("Waiting for share... (%v)", err))
			time.Sleep(5 * time.Second)
			continue
		}

		if err := k.saveShareToFile(share, sessionID); err != nil {
			k.showError("Failed to save share", err)
			return
		}

		k.appendLog(fmt.Sprintf("Successfully received and saved share %d", share.Index))
		k.refreshSharesList()
		break
	}
}

// Helper methods

func (k *KeygenScreen) joinSession(sessionID, password string) error {
	joinData := struct {
		SessionID    string `json:"session_id"`
		ClientID     string `json:"client_id"`
		PasswordHash string `json:"password_hash"`
	}{
		SessionID:    sessionID,
		ClientID:     k.clientID,
		PasswordHash: hashPassword([]byte(password)),
	}

	resp, err := http.Post(
		fmt.Sprintf("%s/join_session", k.apiURL),
		"application/json",
		jsonEncode(joinData),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("%s", string(bodyBytes))
	}

	return nil
}

func (k *KeygenScreen) getShare(sessionID string) (*EncryptedShare, error) {
	resp, err := http.Get(fmt.Sprintf("%s/get_share?session_id=%s&client_id=%s",
		k.apiURL, sessionID, k.clientID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("share not available")
	}

	var shareResp struct {
		Success bool            `json:"success"`
		Share   *EncryptedShare `json:"share"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&shareResp); err != nil {
		return nil, err
	}

	return shareResp.Share, nil
}

func (k *KeygenScreen) splitPrivateKey(privateKey solana.PrivateKey, numShares, threshold int) ([][]byte, error) {
	seed := privateKey[:32]
	return shamir.Split(seed, numShares, threshold)
}

func (k *KeygenScreen) encryptShare(data []byte, password []byte) (string, string, error) {
	key := sha256.Sum256(password)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", "", err
	}

	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}

	ciphertext := aesgcm.Seal(nil, nonce, data, nil)
	return base64.StdEncoding.EncodeToString(ciphertext),
		base64.StdEncoding.EncodeToString(nonce),
		nil
}

func (k *KeygenScreen) saveShareToFile(share *EncryptedShare, sessionID string) error {
	shareFile := ShareFile{
		ShareNumber:    share.Index,
		PublicKey:      share.PublicKey,
		EncryptedShare: share.Data,
		Nonce:          share.Nonce,
		TotalShares:    share.TotalCount,
		Threshold:      share.Threshold,
		CreatedAt:      time.Now().UTC(),
		SessionID:      sessionID,
	}

	shareJSON, err := json.MarshalIndent(shareFile, "", "  ")
	if err != nil {
		return err
	}

	sharesDir := k.getSharesDirectory()
	filename := fmt.Sprintf("%s/share_%d_%s.json", sharesDir, share.Index, share.PublicKey)
	if err := ioutil.WriteFile(filename, shareJSON, 0600); err != nil {
		return err
	}

	// Refresh the shares list after saving
	k.refreshSharesList()
	return nil
}

func (k *KeygenScreen) getSharesDirectory() string {
	rootURI := k.app.Storage().RootURI()
	userDir := rootURI.Path()
	sharesDir := fmt.Sprintf("%s/shares", userDir)
	os.MkdirAll(sharesDir, 0700)
	return sharesDir
}

func (k *KeygenScreen) appendLog(message string) {
	k.logDisplay.SetText(k.logDisplay.Text + message + "\n")
	k.logDisplay.CursorRow = len(strings.Split(k.logDisplay.Text, "\n")) - 1
	k.logDisplay.Refresh()
}

func (k *KeygenScreen) showError(message string, err error) {
	k.appendLog(fmt.Sprintf("Error: %s - %v", message, err))
	dialog.ShowError(fmt.Errorf("%s: %v", message, err), k.window)
}
