package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/taurusgroup/frost-ed25519/pkg/eddsa"
	"github.com/taurusgroup/frost-ed25519/pkg/frost"
	"github.com/taurusgroup/frost-ed25519/pkg/frost/party"
	"github.com/taurusgroup/frost-ed25519/pkg/helpers"
)

type KeyGenOutput struct {
	Secrets map[party.ID]*eddsa.SecretShare
	Shares  *eddsa.Public
}

const (
	rpcURL             = "https://mainnet.helius-rpc.com/?api-key=2c0388dc-a082-4cc5-bad9-29437f3c0715"
	destinationAddress = "StAshdD7TkoNrWqsrbPTwRjCdqaCfMgfVCwKpvaGhuC"
	transferAmount     = 0.0001 * float64(solana.LAMPORTS_PER_SOL)
	apiURL             = "http://localhost:8080"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: go run sign_client.go <session_id> <party_id> <share_file> [initiate]")
		return
	}

	sessionID := os.Args[1]
	partyID := os.Args[2]
	filename := os.Args[3]
	isInitiator := len(os.Args) > 4 && os.Args[4] == "initiate"

	// Load the share file and convert it to KeyGenOutput
	kgOutput, err := loadKeyGenOutput(filename)
	if err != nil {
		fmt.Printf("Error loading key generation output: %v\n", err)
		return
	}

	// Get the total number of parties from the number of shares
	//totalParties := len(kgOutput.Shares.Shares)

	// Join the signing session
	err = joinSigningSession(sessionID, partyID)
	if err != nil {
		fmt.Printf("Error joining signing session: %v\n", err)
		return
	}

	// Wait for the threshold number of parties to join
	err = waitForAllParties(sessionID)
	if err != nil {
		fmt.Printf("Error waiting for parties: %v\n", err)
		return
	}

	// If this is the session initiator, create and broadcast the transaction
	if isInitiator {
		tx, err := createSolanaTransaction(kgOutput.Shares.GroupKey.ToEd25519())
		if err != nil {
			fmt.Printf("Error creating Solana transaction: %v\n", err)
			return
		}
		err = broadcastTransaction(sessionID, tx)
		if err != nil {
			fmt.Printf("Error broadcasting transaction: %v\n", err)
			return
		}
	}

	// If this is not the session initiator, wait a bit to ensure the transaction is broadcasted
	if !isInitiator {
		fmt.Println("Waiting for initiator to broadcast transaction...")
		time.Sleep(5 * time.Second)
	}

	// Perform the signing process
	signature, err := performSigning(sessionID, partyID, kgOutput)
	if err != nil {
		fmt.Printf("Error performing signing: %v\n", err)
		return
	}

	fmt.Printf("Signature: %x\n", signature)

	// If this is the session initiator, finalize the transaction
	if isInitiator {
		err = finalizeTransaction(sessionID, signature)
		if err != nil {
			fmt.Printf("Error finalizing transaction: %v\n", err)
			return
		}
		fmt.Println("Transaction signed and finalized successfully.")
	} else {
		fmt.Println("Signing process completed successfully.")
	}
}

func loadKeyGenOutput(filename string) (*KeyGenOutput, error) {
	jsonData, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var kgOutput KeyGenOutput
	err = json.Unmarshal(jsonData, &kgOutput)
	if err != nil {
		return nil, err
	}
	return &kgOutput, nil
}

func joinSigningSession(sessionID, partyID string) error {
	url := fmt.Sprintf("%s/sign/%s/join", apiURL, sessionID)
	reqBody := map[string]interface{}{
		"partyID": atoi(partyID), // Convert partyID to int
	}
	jsonData, _ := json.Marshal(reqBody)

	fmt.Printf("Sending join request to: %s\n", url)
	fmt.Printf("Request body: %s\n", string(jsonData))

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	fmt.Printf("Response status: %d\n", resp.StatusCode)
	fmt.Printf("Response body: %s\n", string(bodyBytes))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error joining signing session: %s", string(bodyBytes))
	}

	fmt.Println("Successfully joined the signing session")
	return nil
}

func waitForAllParties(sessionID string) error {
	for {
		status, err := getSigningStatus(sessionID)
		if err != nil {
			return err
		}

		if len(status.JoinedParties) >= status.T {
			return nil
		}

		fmt.Printf("Waiting for parties to join (%d/%d, threshold: %d)...\n", len(status.JoinedParties), status.N, status.T)
		time.Sleep(2 * time.Second)
	}
}

func createSolanaTransaction(groupKey []byte) (*solana.Transaction, error) {
	// Convert group key to Solana public key
	fromPubkey := solana.PublicKeyFromBytes(groupKey)
	fmt.Println("From Public Key:", fromPubkey.String())

	// Use the hardcoded destination address
	toPubkey, err := solana.PublicKeyFromBase58(destinationAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid destination address: %v", err)
	}
	fmt.Println("To Public Key:", toPubkey.String())

	// Use the hardcoded transfer amount
	amount := uint64(transferAmount)
	fmt.Printf("Transfer amount: %d lamports (%.9f SOL)\n", amount, float64(amount)/float64(solana.LAMPORTS_PER_SOL))

	instruction := system.NewTransferInstruction(
		amount,
		fromPubkey,
		toPubkey,
	).Build()

	// Get recent blockhash
	client := rpc.New(rpcURL)
	fmt.Println("Fetching recent blockhash...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	recentBlockhash, err := client.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return nil, fmt.Errorf("error fetching recent blockhash: %v", err)
	}
	fmt.Println("Recent blockhash:", recentBlockhash.Value.Blockhash.String())

	tx, err := solana.NewTransaction(
		[]solana.Instruction{instruction},
		recentBlockhash.Value.Blockhash,
		solana.TransactionPayer(fromPubkey),
	)
	if err != nil {
		return nil, fmt.Errorf("error creating transaction: %v", err)
	}

	fmt.Println("Tx:", tx)

	return tx, nil
}

func broadcastTransaction(sessionID string, tx *solana.Transaction) error {
	url := fmt.Sprintf("%s/sign/%s/broadcast", apiURL, sessionID)

	txBytes, err := tx.MarshalBinary()
	if err != nil {
		return fmt.Errorf("error marshaling transaction: %v", err)
	}

	reqBody := map[string]interface{}{
		"transaction": base64.StdEncoding.EncodeToString(txBytes),
	}
	jsonData, _ := json.Marshal(reqBody)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("error broadcasting transaction: %s", string(bodyBytes))
	}

	fmt.Println("Transaction broadcasted successfully")
	return nil
}

func performSigning(sessionID, partyID string, kgOutput *KeyGenOutput) ([]byte, error) {
	currentPartyID := party.ID(atoi(partyID))

	// Wait for the transaction to be broadcasted
	fmt.Println("Waiting for transaction to be broadcasted...")
	for {
		status, err := getSigningStatus(sessionID)
		if err != nil {
			return nil, fmt.Errorf("error getting signing status: %v", err)
		}
		if status.HasTransaction {
			break
		}
		time.Sleep(1 * time.Second)
	}

	// Get the transaction message from the server
	message, err := getTransactionMessage(sessionID)
	if err != nil {
		return nil, fmt.Errorf("error getting transaction message: %v", err)
	}

	fmt.Printf("Received transaction message: %x\n", message)

	// Initialize signing state for this party
	state, output, err := frost.NewSignState(kgOutput.Shares.PartyIDs, kgOutput.Secrets[currentPartyID], kgOutput.Shares, message, 0)
	if err != nil {
		return nil, fmt.Errorf("error creating sign state for party %s: %v", partyID, err)
	}

	// Get the threshold from the signing status
	status, err := getSigningStatus(sessionID)
	if err != nil {
		return nil, fmt.Errorf("error getting signing status: %v", err)
	}
	threshold := status.T

	// Perform MPC signing rounds
	for round := 1; round <= 3; round++ {
		fmt.Printf("Performing MPC signing round %d...\n", round)

		var inMessages [][]byte
		if round > 1 {
			inMessages, err = retrieveMessages(sessionID, partyID, round-1)
			if err != nil {
				return nil, fmt.Errorf("error retrieving messages for round %d: %v", round, err)
			}
			fmt.Printf("Retrieved %d messages for round %d\n", len(inMessages), round-1)
		}

		outMessages, err := helpers.PartyRoutine(inMessages, state)
		if err != nil {
			return nil, fmt.Errorf("error in MPC round %d: %v", round, err)
		}
		fmt.Printf("Generated %d outgoing messages for round %d\n", len(outMessages), round)

		if round < 3 {
			err = submitMessages(sessionID, partyID, round, outMessages)
			if err != nil {
				return nil, fmt.Errorf("error submitting messages for round %d: %v", round, err)
			}
			fmt.Printf("Submitted messages for round %d\n", round)
		}

		// Wait for threshold number of parties to complete this round
		err = waitForRoundCompletion(sessionID, round, threshold)
		if err != nil {
			return nil, fmt.Errorf("error waiting for round completion: %v", err)
		}
		fmt.Printf("Round %d completed\n", round)
	}

	// Get the signature
	sig := output.Signature
	if sig == nil {
		return nil, fmt.Errorf("null signature")
	}

	// Verify the signature
	fmt.Println("Verifying MPC signature...")
	if !kgOutput.Shares.GroupKey.Verify(message, sig) {
		return nil, fmt.Errorf("signature verification failed")
	}
	fmt.Println("MPC signature verified successfully.")

	return sig.ToEd25519(), nil
}

func getTransactionMessage(sessionID string) ([]byte, error) {
	url := fmt.Sprintf("%s/sign/%s/transaction", apiURL, sessionID)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("error getting transaction message: %s", string(bodyBytes))
	}

	var result struct {
		Message string `json:"message"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return base64.StdEncoding.DecodeString(result.Message)
}

func finalizeTransaction(sessionID string, signature []byte) error {
	url := fmt.Sprintf("%s/sign/%s/finalize", apiURL, sessionID)

	reqBody := map[string]interface{}{
		"signature": base64.StdEncoding.EncodeToString(signature),
	}
	jsonData, _ := json.Marshal(reqBody)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("error finalizing transaction: %s", string(bodyBytes))
	}

	fmt.Println("Transaction finalized successfully")
	return nil
}

func retrieveMessages(sessionID, partyID string, round int) ([][]byte, error) {
	url := fmt.Sprintf("http://localhost:8080/sign/%s/messages?partyID=%s&round=%d", sessionID, partyID, round)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("error retrieving messages: %s", string(bodyBytes))
	}

	var result struct {
		Messages []string `json:"messages"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	messages := make([][]byte, len(result.Messages))
	for i, msg := range result.Messages {
		messages[i], err = base64.StdEncoding.DecodeString(msg)
		if err != nil {
			return nil, fmt.Errorf("error decoding message: %v", err)
		}
	}

	return messages, nil
}

func submitMessages(sessionID, partyID string, round int, messages [][]byte) error {
	url := fmt.Sprintf("http://localhost:8080/sign/%s/messages", sessionID)

	type MessageRequest struct {
		To      int    `json:"to"`
		Content string `json:"content"`
	}

	var messageRequests []MessageRequest
	for _, msg := range messages {
		messageRequests = append(messageRequests, MessageRequest{
			To:      0, // Broadcast to all parties
			Content: base64.StdEncoding.EncodeToString(msg),
		})
	}

	reqBody := map[string]interface{}{
		"partyID":  atoi(partyID),
		"round":    round,
		"messages": messageRequests,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("error marshaling request body: %v", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("error submitting messages: %s", string(bodyBytes))
	}

	return nil
}

func waitForRoundCompletion(sessionID string, round, threshold int) error {
	for {
		status, err := getSigningStatus(sessionID)
		if err != nil {
			return err
		}

		messageCount, exists := status.Messages[round]
		if !exists {
			fmt.Printf("Round %d not yet started\n", round)
			time.Sleep(1 * time.Second)
			continue
		}

		fmt.Printf("Waiting for round %d completion: %d/%d messages (threshold: %d)\n", round, messageCount, status.N, threshold)

		if messageCount >= threshold {
			return nil
		}

		time.Sleep(1 * time.Second)
	}
}

func getSigningStatus(sessionID string) (*SigningStatus, error) {
	url := fmt.Sprintf("http://localhost:8080/sign/%s/status", sessionID)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("error getting signing status: %s", string(bodyBytes))
	}

	var status SigningStatus
	err = json.NewDecoder(resp.Body).Decode(&status)
	if err != nil {
		return nil, err
	}

	return &status, nil
}

type SigningStatus struct {
	PartyIDs       []int       `json:"partyIDs"`
	JoinedParties  []int       `json:"joinedParties"`
	Messages       map[int]int `json:"messages"` // Change this to int
	T              int         `json:"t"`
	N              int         `json:"n"`
	HasTransaction bool        `json:"hasTransaction"`
}

// Helper function to convert string to int
func atoi(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}
