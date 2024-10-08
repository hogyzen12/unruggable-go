package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var (
	tokenList     []TokenInfo
	tokenListMu   sync.RWMutex
	tokenListTime time.Time
)

const tokenListURL = "https://tokens.jup.ag/tokens?tags=verified"
const tokenListCacheDuration = 1 * time.Hour
const RPC_ENDPOINT = "http://localhost:3000/api/solana/"

// TokenWidget is a custom widget to display token information
type TokenWidget struct {
	widget.BaseWidget
	Symbol     string
	Balance    float64
	USDBalance float64
	IconURL    string
	container  *fyne.Container
}

func (w *TokenWidget) CreateRenderer() fyne.WidgetRenderer {
	iconSize := fyne.NewSize(24, 24)
	icon := loadImage(w.IconURL, iconSize)

	symbol := widget.NewLabel(w.Symbol)
	symbol.TextStyle = fyne.TextStyle{Bold: true}

	balance := widget.NewLabel(fmt.Sprintf("%.6f", w.Balance))
	usdBalance := widget.NewLabel(fmt.Sprintf("$%.2f", w.USDBalance))

	w.container = container.NewHBox(
		container.NewWithoutLayout(icon),
		container.NewVBox(
			symbol,
			balance,
			usdBalance,
		),
	)

	return widget.NewSimpleRenderer(w.container)
}

func NewHomeScreen() fyne.CanvasObject {
	if err := fetchTokenList(); err != nil {
		fmt.Println("Warning: Failed to fetch token list:", err)
	}

	selectedWallet := GetGlobalState().GetSelectedWallet()
	walletLabel := widget.NewLabel("No wallet selected")
	if selectedWallet != "" {
		walletLabel.SetText(fmt.Sprintf("Loaded Wallet: %s", selectedWallet))
	}

	holdingsLabel := widget.NewLabelWithStyle("Holdings:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	balanceContainer := container.NewVBox()
	scrollContainer := container.NewVScroll(balanceContainer)
	scrollContainer.SetMinSize(fyne.NewSize(300, 400))

	updateBalances := func() {
		response, err := getWalletBalances(selectedWallet)
		if err != nil {
			balanceContainer.Add(widget.NewLabel(fmt.Sprintf("Error fetching balances: %v", err)))
			return
		}

		balanceContainer.Objects = nil // Clear previous balances

		// Display SOL balance
		solHolding := Holding{
			Symbol:     "SOL",
			Balance:    response.SolBalance,
			USDBalance: response.SolBalanceUSD,
			LogoURI:    "https://raw.githubusercontent.com/solana-labs/token-list/main/assets/mainnet/So11111111111111111111111111111111111111112/logo.png",
		}
		solWidget := NewTokenWidget(solHolding)
		balanceContainer.Add(solWidget)
		balanceContainer.Add(widget.NewSeparator())

		// Sort assets by USD balance (descending)
		sort.Slice(response.Assets, func(i, j int) bool {
			return response.Assets[i].USDBalance > response.Assets[j].USDBalance
		})

		// Display other asset balances
		for _, holding := range response.Assets {
			tokenWidget := NewTokenWidget(holding)
			balanceContainer.Add(tokenWidget)
			balanceContainer.Add(widget.NewSeparator())
		}
	}

	updateButton := widget.NewButton("Update Balances", updateBalances)
	updateButton.Importance = widget.HighImportance

	content := container.NewVBox(
		walletLabel,
		holdingsLabel,
		scrollContainer,
		updateButton,
	)

	updateBalances()

	return content
}

func NewTokenWidget(holding Holding) fyne.CanvasObject {
	iconSize := fyne.NewSize(24, 24)
	icon := loadImage(holding.LogoURI, iconSize)

	symbol := widget.NewLabelWithStyle(holding.Symbol, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	leftContent := container.NewHBox(
		container.NewPadded(icon),
		symbol,
	)

	rightContent := container.NewVBox(
		widget.NewLabelWithStyle(fmt.Sprintf("%.6f", holding.Balance), fyne.TextAlignTrailing, fyne.TextStyle{}),
		widget.NewLabelWithStyle(fmt.Sprintf("$%.2f", holding.USDBalance), fyne.TextAlignTrailing, fyne.TextStyle{}),
	)

	return container.NewPadded(
		container.NewBorder(nil, nil, leftContent, rightContent),
	)
}

// Helper function to load and resize images
func loadImage(uriString string, size fyne.Size) fyne.CanvasObject {
	res, err := fyne.LoadResourceFromURLString(uriString)
	if err != nil {
		fmt.Printf("Error loading image from URL: %v\n", err)
		rect := canvas.NewRectangle(theme.DisabledColor())
		rect.Resize(size)
		return rect
	}

	img := canvas.NewImageFromResource(res)
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(size)
	return img
}

// Function to fetch token prices
func fetchTokenList() error {
	tokenListMu.Lock()
	defer tokenListMu.Unlock()

	if time.Since(tokenListTime) < tokenListCacheDuration {
		return nil // Use cached list
	}

	resp, err := http.Get(tokenListURL)
	if err != nil {
		return fmt.Errorf("failed to fetch token list: %v", err)
	}
	defer resp.Body.Close()

	var tokens []TokenInfo
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return fmt.Errorf("failed to decode token list: %v", err)
	}

	tokenList = tokens
	tokenListTime = time.Now()
	return nil
}

func getWalletBalances(walletAddress string) (*WalletResponse, error) {
	resp, err := http.Get(RPC_ENDPOINT + walletAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	var response WalletResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	return &response, nil
}
