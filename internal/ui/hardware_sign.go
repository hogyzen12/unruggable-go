//go:build !js && !wasm
// +build !js,!wasm

package ui

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	confirm "github.com/gagliardetto/solana-go/rpc/sendAndConfirmTransaction"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"github.com/tarm/serial"
)

const (
	RECIPIENT_PUBLIC_KEY = "6tBou5MHL5aWpDy6cgf3wiwGGK2mR8qs68ujtpaoWrf2"
	LAMPORTS_TO_SEND     = 1000000
	SERIAL_PORT          = "/dev/tty.usbserial-0001"
	RPC_URL              = "https://special-blue-fog.solana-mainnet.quiknode.pro/d009d548b4b9dd9f062a8124a868fb915937976c/"
)

// getESP32PublicKey writes "GET_PUBKEY\n" to the serial port and reads the newline‑terminated public key.
func getESP32PublicKey(port *serial.Port) (solana.PublicKey, error) {
	command := "GET_PUBKEY\n"
	_, err := port.Write([]byte(command))
	if err != nil {
		return solana.PublicKey{}, err
	}
	reader := bufio.NewReader(port)
	var pubkeyStr string
	for i := 0; i < 10; i++ {
		line, err := reader.ReadString('\n')
		if err == nil {
			pubkeyStr = line
			break
		}
		time.Sleep(1 * time.Second)
	}
	pubkeyStr = strings.TrimSpace(pubkeyStr)
	if pubkeyStr == "" {
		return solana.PublicKey{}, fmt.Errorf("no public key received from ESP32")
	}
	return solana.PublicKeyFromBase58(pubkeyStr)
}

// createUnsignedTransaction builds a transaction transferring lamports from the ESP32 wallet (as fee payer)
// to the RECIPIENT_PUBLIC_KEY.
func createUnsignedTransaction(client *rpc.Client, esp32Pubkey solana.PublicKey) (*solana.Transaction, error) {
	recipient, err := solana.PublicKeyFromBase58(RECIPIENT_PUBLIC_KEY)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	resp, err := client.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return nil, err
	}
	recentBlockhash := resp.Value.Blockhash

	instr := system.NewTransferInstruction(
		LAMPORTS_TO_SEND,
		esp32Pubkey,
		recipient,
	).Build()

	tx, err := solana.NewTransaction(
		[]solana.Instruction{instr},
		recentBlockhash,
		solana.TransactionPayer(esp32Pubkey),
	)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

// sendToESP32AndGetSignature sends the base64-encoded transaction message over the serial port,
// then waits for a base64-encoded signature response.
func sendToESP32AndGetSignature(port *serial.Port, message string) (string, error) {
	fullMessage := message + "\n"
	_, err := port.Write([]byte(fullMessage))
	if err != nil {
		return "", err
	}
	reader := bufio.NewReader(port)
	var sigStr string
	for i := 0; i < 10; i++ {
		line, err := reader.ReadString('\n')
		if err == nil {
			sigStr = line
			break
		}
		time.Sleep(1 * time.Second)
	}
	sigStr = strings.TrimSpace(sigStr)
	if sigStr == "" {
		return "", fmt.Errorf("no signature received from ESP32")
	}
	return sigStr, nil
}

// signTransaction executes the signing process and updates the provided output widget with log messages.
func signTransaction(output *widget.Entry) {
	// Helper to update the output widget.
	updateOutput := func(s string) {
		// In Fyne v2 many widget methods are thread-safe.
		output.SetText(output.Text + s + "\n")
	}

	updateOutput("Opening serial port...")
	serialConfig := &serial.Config{
		Name:        SERIAL_PORT,
		Baud:        115200,
		ReadTimeout: time.Second * 1,
	}
	port, err := serial.OpenPort(serialConfig)
	if err != nil {
		updateOutput(fmt.Sprintf("Error opening serial port: %v", err))
		return
	}
	defer port.Close()

	updateOutput("Creating RPC client...")
	client := rpc.New(RPC_URL)

	updateOutput("Requesting public key from ESP32...")
	esp32Pubkey, err := getESP32PublicKey(port)
	if err != nil {
		updateOutput(fmt.Sprintf("Error getting ESP32 public key: %v", err))
		return
	}
	updateOutput("Received ESP32 public key: " + esp32Pubkey.String())

	updateOutput("Creating unsigned transaction...")
	tx, err := createUnsignedTransaction(client, esp32Pubkey)
	if err != nil {
		updateOutput(fmt.Sprintf("Error creating transaction: %v", err))
		return
	}

	msgBytes, err := tx.Message.MarshalBinary()
	if err != nil {
		updateOutput(fmt.Sprintf("Error serializing message: %v", err))
		return
	}
	base64Message := base64.StdEncoding.EncodeToString(msgBytes)
	updateOutput("Serialized Transaction Message (Base64): " + base64Message)

	updateOutput("Sending message to ESP32 for signature...")
	base64Signature, err := sendToESP32AndGetSignature(port, base64Message)
	if err != nil {
		updateOutput(fmt.Sprintf("Error receiving signature: %v", err))
		return
	}
	updateOutput("Received signature from ESP32: " + base64Signature)

	sigBytes, err := base64.StdEncoding.DecodeString(base64Signature)
	if err != nil {
		updateOutput(fmt.Sprintf("Error decoding signature: %v", err))
		return
	}
	var signature solana.Signature
	copy(signature[:], sigBytes)
	tx.Signatures = []solana.Signature{signature}

	updateOutput("Connecting to WS for transaction confirmation...")
	wsClient, err := ws.Connect(context.Background(), "wss://api.mainnet-beta.solana.com")
	if err != nil {
		updateOutput(fmt.Sprintf("Error connecting to WS: %v", err))
		return
	}
	defer wsClient.Close()

	updateOutput("Sending transaction and waiting for confirmation...")
	sig, err := confirm.SendAndConfirmTransaction(context.Background(), client, wsClient, tx)
	if err != nil {
		updateOutput(fmt.Sprintf("Error sending transaction: %v", err))
		return
	}
	updateOutput("Transaction submitted with signature: " + sig.String())
}

// NewSignScreen creates a new UI screen containing a button that runs the signing code.
func NewSignScreen() fyne.CanvasObject {
	output := widget.NewMultiLineEntry()
	output.SetPlaceHolder("Output logs will appear here...")
	output.Disable() // Make the entry read-only

	signButton := widget.NewButton("Sign and Send Transaction", func() {
		go signTransaction(output)
	})

	return container.NewVBox(signButton, output)
}
