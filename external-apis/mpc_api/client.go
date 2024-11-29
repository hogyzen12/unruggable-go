package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/taurusgroup/frost-ed25519/pkg/frost"
	"github.com/taurusgroup/frost-ed25519/pkg/frost/party"
	"github.com/taurusgroup/frost-ed25519/pkg/helpers"
)

type Secret struct {
	ID     int    `json:"id"`
	Secret string `json:"secret"`
}

type Shares struct {
	T        int               `json:"t"`
	GroupKey string            `json:"groupkey"`
	Shares   map[string]string `json:"shares"`
}

type Share struct {
	Secret Secret `json:"Secret"`
	Shares Shares `json:"Shares"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run client.go <sessionID>")
		return
	}

	sessionID := os.Args[1]
	apiURL := "http://localhost:8080"
	fmt.Printf("Using apiURL: %s\n", apiURL) // Add this line

	// Join the session and retrieve session info
	partyID, partyIDs, t, err := joinAndRetrieveSessionInfo(apiURL, sessionID)
	if err != nil {
		fmt.Printf("Error retrieving session info: %v\n", err)
		return
	}

	fmt.Printf("Joined session as party %d with parties %v and threshold %d\n", partyID, partyIDs, t)

	// Initialize the party's state and output
	s, output, err := frost.NewKeygenState(partyID, partyIDs, party.Size(t), 0)
	if err != nil {
		fmt.Printf("Error initializing state: %v\n", err)
		return
	}

	// Round 1
	msgsOut1, err := helpers.PartyRoutine(nil, s)
	if err != nil {
		fmt.Printf("Error in Round 1: %v\n", err)
		return
	}
	// All messages are broadcasts in Round 1
	recipients1 := make([]int, len(msgsOut1))
	for i := range recipients1 {
		recipients1[i] = 0 // 0 indicates broadcast
	}
	submitMessages(apiURL, sessionID, int(partyID), 1, msgsOut1, recipients1)

	msgsIn1 := retrieveMessages(apiURL, sessionID, int(partyID), 1)
	msgsOut2, err := helpers.PartyRoutine(msgsIn1, s)
	if err != nil {
		fmt.Printf("Error handling messages in Round 1: %v\n", err)
		return
	}

	// Round 2
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

	submitMessages(apiURL, sessionID, int(partyID), 2, msgsOut2Bytes, recipients2)

	msgsIn2 := retrieveMessages(apiURL, sessionID, int(partyID), 2)
	_, err = helpers.PartyRoutine(msgsIn2, s)
	if err != nil {
		fmt.Printf("Error handling messages in Round 2: %v\n", err)
		return
	}

	// Wait for completion
	if err := s.WaitForError(); err != nil {
		fmt.Printf("Error during key generation: %v\n", err)
		return
	}

	// Display the results
	public := output.Public
	groupKey := public.GroupKey.ToEd25519()
	secretShare := output.SecretKey.Secret.Bytes()
	publicShare := public.Shares[partyID].Bytes()

	fmt.Printf("Group Key: %x\n", groupKey)
	fmt.Printf("Secret Share: %x\n", secretShare)
	fmt.Printf("Public Share: %x\n", publicShare)

	// After key generation, update the share saving logic:
	share := Share{
		Secret: Secret{
			ID:     int(partyID),
			Secret: base64.StdEncoding.EncodeToString(secretShare),
		},
		Shares: Shares{
			T:        t,
			GroupKey: base64.StdEncoding.EncodeToString(groupKey),
			Shares:   make(map[string]string),
		},
	}

	// Populate the Shares.Shares map
	for id, partyShare := range public.Shares {
		share.Shares.Shares[fmt.Sprintf("%d", id)] = base64.StdEncoding.EncodeToString(partyShare.Bytes())
	}

	// Create the outer structure
	outerShare := make(map[string]interface{})
	outerShare["Secret"] = map[string]Secret{
		fmt.Sprintf("%d", partyID): share.Secret,
	}
	outerShare["Shares"] = share.Shares

	fileData, err := json.MarshalIndent(outerShare, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling share data: %v\n", err)
		return
	}

	os.MkdirAll("shares", 0700)
	filename := fmt.Sprintf("shares/party-%d-share.json", partyID)
	err = ioutil.WriteFile(filename, fileData, 0600)
	if err != nil {
		fmt.Printf("Error writing share to file: %v\n", err)
		return
	}

	fmt.Printf("Share saved to %s\n", filename)
}

func joinAndRetrieveSessionInfo(apiURL, sessionID string) (party.ID, []party.ID, int, error) {
	// Implementation remains the same
	url := fmt.Sprintf("%s/keygen/%s/join", apiURL, sessionID)
	fmt.Printf("Joining session at URL: %s\n", url) // Add this line
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

		fmt.Println("Waiting for other parties to join...")
		time.Sleep(2 * time.Second)
	}

	return partyID, partyIDs, t, nil
}

func submitMessages(apiURL, sessionID string, partyID, round int, messages [][]byte, recipients []int) {
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
		fmt.Printf("Error submitting messages: %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Printf("Submit Messages Response: %s\n", body)
}

func retrieveMessages(apiURL, sessionID string, partyID, round int) [][]byte {
	url := fmt.Sprintf("%s/keygen/%s/messages?partyID=%d&round=%d", apiURL, sessionID, partyID, round)
	for {
		resp, err := http.Get(url)
		if err != nil {
			fmt.Printf("Error retrieving messages: %v\n", err)
			time.Sleep(2 * time.Second)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			var result struct {
				Messages []string `json:"messages"`
			}
			err = json.NewDecoder(resp.Body).Decode(&result)
			if err != nil {
				fmt.Printf("Error decoding messages: %v\n", err)
				return nil
			}

			var messages [][]byte
			for _, msgStr := range result.Messages {
				msgBytes, err := base64.StdEncoding.DecodeString(msgStr)
				if err != nil {
					fmt.Printf("Error decoding message: %v\n", err)
					return nil
				}
				messages = append(messages, msgBytes)
			}

			fmt.Printf("Retrieved Messages: %v\n", messages)
			return messages
		} else {
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			fmt.Printf("Waiting for messages: %s\n", string(bodyBytes))
			time.Sleep(2 * time.Second)
		}
	}
}
