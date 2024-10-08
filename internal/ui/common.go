package ui

import (
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
