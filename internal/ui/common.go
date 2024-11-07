package ui

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
)

type TokenInfo struct {
	Address  string          `json:"address"`
	Symbol   string          `json:"symbol"`
	Decimals int             `json:"decimals"`
	PriceUSD float64         `json:"priceUSD"` // Store token price in USD if available
	Balance  json.RawMessage `json:"balance"`  // Raw balance (can be string or number)
}

type AssetInfo struct {
	ID        string    `json:"id"`
	TokenInfo TokenInfo `json:"token_info"`
}

type AssetsResponse struct {
	Result struct {
		Items         []AssetInfo `json:"items"`
		NativeBalance struct {
			Lamports int64 `json:"lamports"`
		} `json:"nativeBalance"`
	} `json:"result"`
}

type Holding struct {
	Symbol     string  `json:"symbol"`
	Address    string  `json:"address"`
	Balance    float64 `json:"balance"`
	USDPrice   float64 `json:"usdPrice"`
	USDBalance float64 `json:"usdBalance"`
	LogoURI    string  `json:"logoURI"`
}

type WalletResponse struct {
	SolBalance    float64   `json:"solBalance"`
	SolBalanceUSD float64   `json:"solBalanceUSD"`
	Assets        []Holding `json:"assets"`
}

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

func hashPassword(password []byte) string {
	hash := sha256.Sum256(password)
	return hex.EncodeToString(hash[:])
}

func jsonEncode(v interface{}) *bytes.Buffer {
	buf := new(bytes.Buffer)
	json.NewEncoder(buf).Encode(v)
	return buf
}
