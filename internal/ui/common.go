package ui

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Token represents a token from the Jupiter token list
type Token struct {
	Address string `json:"address"`
	Symbol  string `json:"symbol"`
	Name    string `json:"name"`
	LogoURI string `json:"logoURI"`
}

// TokenInfo represents token details from the RPC response
type TokenInfo struct {
	Symbol    string          `json:"symbol"`
	Decimals  int             `json:"decimals"`
	Balance   json.RawMessage `json:"balance"`
	PriceInfo struct {
		PricePerToken float64 `json:"price_per_token"`
	} `json:"price_info"`
}

// AssetInfo represents an individual asset from the RPC response
type AssetInfo struct {
	ID        string    `json:"id"`
	TokenInfo TokenInfo `json:"token_info"`
}

// AssetsResponse represents the Helius RPC getAssetsByOwner response
type AssetsResponse struct {
	NativeBalance struct {
		Lamports    int64   `json:"lamports"`
		PricePerSol float64 `json:"price_per_sol"`
	} `json:"nativeBalance"`
	Items []AssetInfo `json:"items"`
}

// CalypsoAssetsResponse represents an alternative assets response structure
type CalypsoAssetsResponse struct {
	Result struct {
		Items         []AssetInfo `json:"items"`
		NativeBalance struct {
			Lamports    int64   `json:"lamports"`
			PricePerSol float64 `json:"price_per_sol"`
		} `json:"nativeBalance"`
	} `json:"result"`
}

// Holding represents a wallet holding
type Holding struct {
	Symbol     string  `json:"symbol"`
	Address    string  `json:"address"`
	Balance    float64 `json:"balance"`
	USDPrice   float64 `json:"usdPrice"`
	USDBalance float64 `json:"usdBalance"`
	Decimals   int     `json:"decimals"` // Added for SPL token transfers
}

// WalletResponse represents the wallet balance response
type WalletResponse struct {
	SolBalance    float64   `json:"solBalance"`
	SolBalanceUSD float64   `json:"solBalanceUSD"`
	Assets        []Holding `json:"assets"`
}

// decryptShare decrypts an encrypted share using a password
func decryptShare(encryptedData string, nonceStr string, password []byte) ([]byte, error) {
	key := sha256.Sum256(password)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return nil, err
	}

	nonce, err := base64.StdEncoding.DecodeString(nonceStr)
	if err != nil {
		return nil, err
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// hashPassword generates a SHA-256 hash of a password
func hashPassword(password []byte) string {
	hash := sha256.Sum256(password)
	return hex.EncodeToString(hash[:])
}

// jsonEncode encodes a value to JSON and returns it as a buffer
func jsonEncode(v interface{}) *bytes.Buffer {
	buf := new(bytes.Buffer)
	json.NewEncoder(buf).Encode(v)
	return buf
}

// padKey pads a key to 32 bytes
func padKey(key string) string {
	for len(key) < 32 {
		key += key
	}
	return key[:32]
}

func shortenAddress(address string) string {
	if len(address) <= 8 {
		return address
	}
	return fmt.Sprintf("%s...%s", address[:4], address[len(address)-4:])
}
