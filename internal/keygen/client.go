package keygen

import (
    "bytes"
    "encoding/base64"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "net/http"
    "os"
    "strings"
    "time"

    "github.com/taurusgroup/frost-ed25519/pkg/frost"
    "github.com/taurusgroup/frost-ed25519/pkg/frost/party"
    "github.com/taurusgroup/frost-ed25519/pkg/helpers"
)

type Share struct {
    PartyID     int    `json:"party_id"`
    GroupKey    string `json:"group_key"`    // hex encoded
    SecretShare string `json:"secret_share"` // hex encoded
    PublicShare string `json:"public_share"` // hex encoded
}

func PerformKeyGeneration(apiURL, sessionID string, t, n int, statusCallback func(string)) error {
    // Trim any trailing slashes from apiURL
    apiURL = strings.TrimRight(apiURL, "/")

    // Join the session and retrieve session info
    partyID, partyIDs, threshold, err := joinAndRetrieveSessionInfo(apiURL, sessionID, t, n, statusCallback)
    if err != nil {
        return fmt.Errorf("error retrieving session info: %v", err)
    }

    statusCallback(fmt.Sprintf("Joined session as party %d with parties %v and threshold %d", partyID, partyIDs, threshold))

    // Initialize the party's state and output
    s, output, err := frost.NewKeygenState(partyID, partyIDs, party.Size(threshold), 0)
    if err != nil {
        return fmt.Errorf("error initializing state: %v", err)
    }

    // Round 1
    statusCallback("Starting Round 1")
    msgsOut1, err := helpers.PartyRoutine(nil, s)
    if err != nil {
        return fmt.Errorf("error in Round 1: %v", err)
    }
    recipients1 := make([]int, len(msgsOut1))
    for i := range recipients1 {
        recipients1[i] = 0 // 0 indicates broadcast
    }
    submitMessages(apiURL, sessionID, int(partyID), 1, msgsOut1, recipients1)

    msgsIn1 := retrieveMessages(apiURL, sessionID, int(partyID), 1, statusCallback)
    msgsOut2, err := helpers.PartyRoutine(msgsIn1, s)
    if err != nil {
        return fmt.Errorf("error handling messages in Round 1: %v", err)
    }

    // Round 2
    statusCallback("Starting Round 2")
    var msgsOut2Bytes [][]byte
    var recipients2 []int

    msgIndex := 0
    for _, otherPartyID := range partyIDs {
        if otherPartyID == partyID {
            continue
        }
        msgBytes := msgsOut2[msgIndex]
        msgsOut2Bytes = append(msgsOut2Bytes, msgBytes)
        recipients2 = append(recipients2, int(otherPartyID))
        msgIndex++
    }

    submitMessages(apiURL, sessionID, int(partyID), 2, msgsOut2Bytes, recipients2)

    msgsIn2 := retrieveMessages(apiURL, sessionID, int(partyID), 2, statusCallback)
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

    statusCallback(fmt.Sprintf("Group Key: %x", groupKey))
    statusCallback(fmt.Sprintf("Secret Share: %x", secretShare))
    statusCallback(fmt.Sprintf("Public Share: %x", publicShare))

    // Save the shares to a file
    share := Share{
        PartyID:     int(partyID),
        GroupKey:    hex.EncodeToString(groupKey),
        SecretShare: hex.EncodeToString(secretShare),
        PublicShare: hex.EncodeToString(publicShare),
    }

    fileData, err := json.MarshalIndent(share, "", "  ")
    if err != nil {
        return fmt.Errorf("error marshaling share data: %v", err)
    }

    os.MkdirAll("shares", 0700)
    filename := fmt.Sprintf("shares/party-%d-share.json", partyID)
    err = ioutil.WriteFile(filename, fileData, 0600)
    if err != nil {
        return fmt.Errorf("error writing share to file: %v", err)
    }

    statusCallback(fmt.Sprintf("Share saved to %s", filename))

    return nil
}

// The rest of the helper functions (joinAndRetrieveSessionInfo, submitMessages, retrieveMessages)
// need to be adapted slightly to accept the statusCallback function.

func joinAndRetrieveSessionInfo(apiURL, sessionID string, t, n int, statusCallback func(string)) (party.ID, []party.ID, int, error) {
    // Implementation adjusted to initiate the session if necessary
    // Attempt to join the session
    url := fmt.Sprintf("%s/keygen/%s/join", apiURL, sessionID)
    statusCallback(fmt.Sprintf("Joining session at URL: %s", url))

    reqBody := map[string]interface{}{}
    jsonData, _ := json.Marshal(reqBody)
    resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
    if err != nil {
        return 0, nil, 0, fmt.Errorf("error joining session: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        // If session not found, try to initiate it
        if resp.StatusCode == http.StatusNotFound {
            statusCallback("Session not found, attempting to initiate a new session")
            if err := initiateSession(apiURL, sessionID, t, n, statusCallback); err != nil {
                return 0, nil, 0, err
            }
            // Retry joining
            return joinAndRetrieveSessionInfo(apiURL, sessionID, t, n, statusCallback)
        }
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
    threshold := joinResult.T
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

        statusCallback(fmt.Sprintf("Joined parties: %v/%v", len(statusResult.JoinedParties), N))

        if len(statusResult.JoinedParties) >= N {
            // All parties have joined
            for _, id := range statusResult.PartyIDs {
                partyIDs = append(partyIDs, party.ID(id))
            }
            break
        }

        statusCallback("Waiting for other parties to join...")
        time.Sleep(2 * time.Second)
    }

    return partyID, partyIDs, threshold, nil
}

func initiateSession(apiURL, sessionID string, t, n int, statusCallback func(string)) error {
    url := fmt.Sprintf("%s/keygen/initiate", apiURL)
    reqBody := map[string]interface{}{
        "t": t,
        "n": n,
    }
    jsonData, _ := json.Marshal(reqBody)
    resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
    if err != nil {
        return fmt.Errorf("error initiating session: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        bodyBytes, _ := ioutil.ReadAll(resp.Body)
        return fmt.Errorf("error initiating session: %s", string(bodyBytes))
    }

    statusCallback("Session initiated successfully")
    return nil
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

func retrieveMessages(apiURL, sessionID string, partyID, round int, statusCallback func(string)) [][]byte {
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

            statusCallback(fmt.Sprintf("Retrieved Messages: %v", messages))
            return messages
        } else {
            bodyBytes, _ := ioutil.ReadAll(resp.Body)
            statusCallback(fmt.Sprintf("Waiting for messages: %s", string(bodyBytes)))
            time.Sleep(2 * time.Second)
        }
    }
}
