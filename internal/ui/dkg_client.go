package ui

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
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
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/mr-tron/base58"
	"github.com/taurusgroup/frost-ed25519/pkg/frost"
	"github.com/taurusgroup/frost-ed25519/pkg/frost/party"
	"github.com/taurusgroup/frost-ed25519/pkg/helpers"
)

type LogFunc func(message string)

func initiateSession(apiURL string, t, n int) (string, error) {
	url := fmt.Sprintf("%s/keygen/initiate", apiURL)
	reqBody := map[string]interface{}{
		"t": t,
		"n": n,
	}
	jsonData, _ := json.Marshal(reqBody)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("error initiating session: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		Message   string `json:"message"`
		SessionID string `json:"sessionID"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", fmt.Errorf("error decoding initiate response: %v", err)
	}

	return result.SessionID, nil
}

func joinAndRetrieveSessionInfo(apiURL, sessionID string, log LogFunc) (party.ID, []party.ID, int, error) {
	url := fmt.Sprintf("%s/keygen/%s/join", apiURL, sessionID)
	log(fmt.Sprintf("Joining session at URL: %s", url))
	reqBody := map[string]interface{}{}
	jsonData, _ := json.Marshal(reqBody)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, nil, 0, fmt.Errorf("error joining session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return 0, nil, 0, fmt.Errorf("error joining session: %s", string(bodyBytes))
	}

	var joinResult struct {
		Message string `json:"message"`
		PartyID int    `json:"partyID"`
		T       int    `json:"t"`
		N       int    `json:"n"`
	}
	err = json.NewDecoder(resp.Body).Decode(&joinResult)
	if err != nil {
		return 0, nil, 0, fmt.Errorf("error decoding join response: %v", err)
	}

	partyID := party.ID(joinResult.PartyID)
	t := joinResult.T
	N := joinResult.N

	// Wait until all parties have joined
	var partyIDs []party.ID
	for {
		url := fmt.Sprintf("%s/keygen/%s/status", apiURL, sessionID)
		resp, err := http.Get(url)
		if err != nil {
			return 0, nil, 0, fmt.Errorf("error getting session status: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			return 0, nil, 0, fmt.Errorf("error getting session status: %s", string(bodyBytes))
		}
		var statusResult struct {
			PartyIDs      []int `json:"partyIDs"`
			JoinedParties []int `json:"joinedParties"`
			T             int   `json:"t"`
			N             int   `json:"n"`
		}
		err = json.NewDecoder(resp.Body).Decode(&statusResult)
		if err != nil {
			return 0, nil, 0, fmt.Errorf("error decoding status response: %v", err)
		}

		if len(statusResult.JoinedParties) >= N {
			// All parties have joined
			for _, id := range statusResult.PartyIDs {
				partyIDs = append(partyIDs, party.ID(id))
			}
			break
		}

		log("Waiting for other parties to join...")
		time.Sleep(2 * time.Second)
	}

	return partyID, partyIDs, t, nil
}

func performKeyGeneration(apiURL, sessionID string, partyID party.ID, partyIDs []party.ID, t int, log LogFunc, window fyne.Window, app fyne.App) error {
	// Initialize the party's state and output
	s, output, err := frost.NewKeygenState(partyID, partyIDs, party.Size(t), 0)
	if err != nil {
		return fmt.Errorf("error initializing state: %v", err)
	}

	// Round 1
	log("Starting Round 1")
	msgsOut1, err := helpers.PartyRoutine(nil, s)
	if err != nil {
		return fmt.Errorf("error in Round 1: %v", err)
	}
	// All messages are broadcasts in Round 1
	recipients1 := make([]int, len(msgsOut1))
	for i := range recipients1 {
		recipients1[i] = 0 // 0 indicates broadcast
	}
	submitMessages(apiURL, sessionID, int(partyID), 1, msgsOut1, recipients1, log)

	log("Retrieving messages for Round 1")
	msgsIn1 := retrieveMessages(apiURL, sessionID, int(partyID), 1, log)
	msgsOut2, err := helpers.PartyRoutine(msgsIn1, s)
	if err != nil {
		return fmt.Errorf("error handling messages in Round 1: %v", err)
	}

	// Round 2
	log("Starting Round 2")
	// Prepare messages and recipients
	var msgsOut2Bytes [][]byte
	var recipients2 []int

	// Each message in msgsOut2 corresponds to a recipient
	msgIndex := 0
	for _, otherPartyID := range partyIDs {
		if otherPartyID == partyID {
			continue
		}
		// Collect the message for this recipient
		msgBytes := msgsOut2[msgIndex]
		msgsOut2Bytes = append(msgsOut2Bytes, msgBytes)
		recipients2 = append(recipients2, int(otherPartyID))
		msgIndex++
	}

	submitMessages(apiURL, sessionID, int(partyID), 2, msgsOut2Bytes, recipients2, log)

	log("Retrieving messages for Round 2")
	msgsIn2 := retrieveMessages(apiURL, sessionID, int(partyID), 2, log)
	_, err = helpers.PartyRoutine(msgsIn2, s)
	if err != nil {
		return fmt.Errorf("error handling messages in Round 2: %v", err)
	}

	// Wait for completion
	if err := s.WaitForError(); err != nil {
		return fmt.Errorf("error during key generation: %v", err)
	}

	// Display the results
	public := output.Public
	groupKey := public.GroupKey.ToEd25519()
	secretShare := output.SecretKey.Secret.Bytes()
	publicShare := public.Shares[partyID].Bytes()
	groupKeyBase58 := base58.Encode(groupKey)

	log(fmt.Sprintf("Group Key: %x", groupKey))
	log(fmt.Sprintf("Solana Group Key: %x", groupKeyBase58))
	log(fmt.Sprintf("Secret Share: %x", secretShare))
	log(fmt.Sprintf("Public Share: %x", publicShare))

	// Save the shares
	saveShares(groupKeyBase58, secretShare, publicShare, int(partyID), window, app, log)

	return nil
}

func submitMessages(apiURL, sessionID string, partyID, round int, messages [][]byte, recipients []int, log LogFunc) {
	url := fmt.Sprintf("%s/keygen/%s/messages", apiURL, sessionID)

	var messagesToSend []map[string]interface{}
	for i, msgContent := range messages {
		msgMap := map[string]interface{}{
			"to":      recipients[i],
			"content": base64.StdEncoding.EncodeToString(msgContent),
		}
		messagesToSend = append(messagesToSend, msgMap)
	}

	reqBody := map[string]interface{}{
		"partyID":  partyID,
		"round":    round,
		"messages": messagesToSend,
	}
	jsonData, _ := json.Marshal(reqBody)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log(fmt.Sprintf("Error submitting messages: %v", err))
		return
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	log(fmt.Sprintf("Submit Messages Response: %s", body))
}

func retrieveMessages(apiURL, sessionID string, partyID, round int, log LogFunc) [][]byte {
	url := fmt.Sprintf("%s/keygen/%s/messages?partyID=%d&round=%d", apiURL, sessionID, partyID, round)
	for {
		resp, err := http.Get(url)
		if err != nil {
			log(fmt.Sprintf("Error retrieving messages: %v", err))
			time.Sleep(2 * time.Second)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			var result struct {
				Messages []string `json:"messages"`
			}
			err = json.NewDecoder(resp.Body).Decode(&result)
			if err != nil {
				log(fmt.Sprintf("Error decoding messages: %v", err))
				return nil
			}

			var messages [][]byte
			for _, msgStr := range result.Messages {
				msgBytes, err := base64.StdEncoding.DecodeString(msgStr)
				if err != nil {
					log(fmt.Sprintf("Error decoding message: %v", err))
					return nil
				}
				messages = append(messages, msgBytes)
			}

			log(fmt.Sprintf("Retrieved %d messages for round %d", len(messages), round))
			return messages
		} else {
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			log(fmt.Sprintf("Waiting for messages: %s", string(bodyBytes)))
			time.Sleep(2 * time.Second)
		}
	}
}

func saveShares(groupKeyBase58 string, secretShare, publicShare []byte, partyID int, window fyne.Window, app fyne.App, log LogFunc) {
	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Enter password to encrypt secret share")

	dialog.ShowCustomConfirm("Encrypt Secret Share", "Save", "Cancel", passwordEntry, func(encrypt bool) {
		if encrypt {
			password := passwordEntry.Text
			if password == "" {
				dialog.ShowError(fmt.Errorf("password cannot be empty"), window)
				return
			}

			err := saveEncryptedShares(groupKeyBase58, secretShare, publicShare, partyID, password, app)
			if err != nil {
				dialog.ShowError(fmt.Errorf("failed to save shares: %v", err), window)
				return
			}

			log(fmt.Sprintf("Shares saved successfully for group key: %s", groupKeyBase58))
			dialog.ShowInformation("Shares Saved", "Shares have been securely stored", window)
		}
	}, window)
}

func saveEncryptedShares(groupKeyBase58 string, secretShare, publicShare []byte, partyID int, password string, app fyne.App) error {
	encryptedSecretShare, err := encrypt(secretShare, password)
	if err != nil {
		return err
	}

	shares := SharesData{
		GroupKey:    groupKeyBase58,
		PartyID:     partyID,
		SecretShare: encryptedSecretShare,
		PublicShare: hex.EncodeToString(publicShare),
	}

	jsonData, err := json.Marshal(shares)
	if err != nil {
		return err
	}

	sharesDir := getSharesDirectory(app)
	filename := filepath.Join(sharesDir, fmt.Sprintf("%s_party%d.share", groupKeyBase58, partyID))

	return ioutil.WriteFile(filename, jsonData, 0600)
}

type SharesData struct {
	GroupKey    string `json:"groupKey"`
	PartyID     int    `json:"partyID"`
	SecretShare string `json:"secretShare"`
	PublicShare string `json:"publicShare"`
}

func getSharesDirectory(app fyne.App) string {
	rootURI := app.Storage().RootURI()
	userDir := rootURI.Path()

	sharesDir := filepath.Join(userDir, "shares")
	if _, err := os.Stat(sharesDir); os.IsNotExist(err) {
		os.MkdirAll(sharesDir, 0700)
	}

	return sharesDir
}

func GetSavedShares(app fyne.App) ([]SharesData, error) {
	sharesDir := getSharesDirectory(app)
	files, err := ioutil.ReadDir(sharesDir)
	if err != nil {
		return nil, err
	}

	var shares []SharesData
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".share") {
			data, err := ioutil.ReadFile(filepath.Join(sharesDir, file.Name()))
			if err != nil {
				continue
			}

			var share SharesData
			if err := json.Unmarshal(data, &share); err != nil {
				continue
			}

			shares = append(shares, share)
		}
	}

	return shares, nil
}

func encrypt_share(data []byte, passphrase string) (string, error) {
	block, _ := aes.NewCipher([]byte(padKeyShare(passphrase)))
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return hex.EncodeToString(ciphertext), nil
}

func padKeyShare(key string) string {
	for len(key) < 32 {
		key += key
	}
	return key[:32]
}
