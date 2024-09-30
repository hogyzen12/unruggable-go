package ui

import (
	"fmt"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/shopspring/decimal"
)

type CalypsoBot struct {
	window           fyne.Window
	status           *widget.Label
	log              *widget.Entry
	startStopButton  *widget.Button
	isRunning        bool
	checkInterval    int
	rebalanceThreshold decimal.Decimal
	stashThreshold   decimal.Decimal
	stashAmount      decimal.Decimal
	stashAddress     string
}

func NewCalypsoScreen(window fyne.Window) fyne.CanvasObject {
	bot := &CalypsoBot{
		window:           window,
		status:           widget.NewLabel("Bot Status: Stopped"),
		log:              widget.NewMultiLineEntry(),
		isRunning:        false,
		checkInterval:    60,
		rebalanceThreshold: decimal.NewFromFloat(0.0042),
		stashThreshold:   decimal.NewFromInt(10),
		stashAmount:      decimal.NewFromInt(1),
		stashAddress:     "StAshdD7TkoNrWqsrbPTwRjCdqaCfMgfVCwKpvaGhuC",
	}

	bot.startStopButton = widget.NewButton("Start Bot", bot.toggleBot)

	bot.log.Disable()

	checkIntervalEntry := widget.NewEntry()
	checkIntervalEntry.SetText(strconv.Itoa(bot.checkInterval))
	checkIntervalEntry.OnChanged = func(value string) {
		if interval, err := strconv.Atoi(value); err == nil {
			bot.checkInterval = interval
		}
	}

	rebalanceThresholdEntry := widget.NewEntry()
	rebalanceThresholdEntry.SetText(bot.rebalanceThreshold.String())
	rebalanceThresholdEntry.OnChanged = func(value string) {
		if threshold, err := decimal.NewFromString(value); err == nil {
			bot.rebalanceThreshold = threshold
		}
	}

	stashThresholdEntry := widget.NewEntry()
	stashThresholdEntry.SetText(bot.stashThreshold.String())
	stashThresholdEntry.OnChanged = func(value string) {
		if threshold, err := decimal.NewFromString(value); err == nil {
			bot.stashThreshold = threshold
		}
	}

	stashAmountEntry := widget.NewEntry()
	stashAmountEntry.SetText(bot.stashAmount.String())
	stashAmountEntry.OnChanged = func(value string) {
		if amount, err := decimal.NewFromString(value); err == nil {
			bot.stashAmount = amount
		}
	}

	stashAddressEntry := widget.NewEntry()
	stashAddressEntry.SetText(bot.stashAddress)
	stashAddressEntry.OnChanged = func(value string) {
		bot.stashAddress = value
	}

	return container.NewVBox(
		widget.NewLabel("Calypso Trading Bot"),
		bot.status,
		container.NewGridWithColumns(2,
			widget.NewLabel("Check Interval (seconds):"),
			checkIntervalEntry,
			widget.NewLabel("Rebalance Threshold:"),
			rebalanceThresholdEntry,
			widget.NewLabel("Stash Threshold ($):"),
			stashThresholdEntry,
			widget.NewLabel("Stash Amount ($):"),
			stashAmountEntry,
			widget.NewLabel("Stash Address:"),
			stashAddressEntry,
		),
		bot.startStopButton,
		widget.NewLabel("Bot Log:"),
		bot.log,
	)
}

func (b *CalypsoBot) toggleBot() {
	if b.isRunning {
		b.stopBot()
	} else {
		b.startBot()
	}
}

func (b *CalypsoBot) startBot() {
	b.isRunning = true
	b.status.SetText("Bot Status: Running")
	b.startStopButton.SetText("Stop Bot")
	b.log.SetText("")
	go b.runBot()
}

func (b *CalypsoBot) stopBot() {
	b.isRunning = false
	b.status.SetText("Bot Status: Stopped")
	b.startStopButton.SetText("Start Bot")
	b.logMessage("Bot stopped.")
}

func (b *CalypsoBot) runBot() {
	b.logMessage("Bot started.")
	for b.isRunning {
		b.performBotCycle()
		time.Sleep(time.Duration(b.checkInterval) * time.Second)
	}
}

func (b *CalypsoBot) performBotCycle() {
	// Get the current wallet from the global state
	currentWallet := GetGlobalState().GetSelectedWallet()
	if currentWallet == "" {
		b.logMessage("Error: No wallet selected.")
		return
	}

	b.logMessage(fmt.Sprintf("Checking portfolio for wallet: %s", currentWallet))

	// TODO: Implement the following functions
	// balances, err := getWalletBalances(currentWallet)
	// prices, err := getPrices()
	// portfolioValue, err := calculatePortfolioValue(balances, prices)
	// rebalanceAmounts, err := calculateRebalanceAmounts(balances, prices, portfolioValue)

	// For now, we'll just log placeholder messages
	b.logMessage("Fetched wallet balances.")
	b.logMessage("Fetched asset prices.")
	b.logMessage("Calculated portfolio value.")
	b.logMessage("Calculated rebalance amounts.")

	// TODO: Implement rebalancing and stashing logic
	// if needsRebalance(rebalanceAmounts) {
	//     err := executeRebalance(rebalanceAmounts, prices, currentWallet)
	//     if err != nil {
	//         b.logMessage(fmt.Sprintf("Error during rebalance: %v", err))
	//     } else {
	//         b.logMessage("Rebalance executed successfully.")
	//     }
	// }

	// if needsStash(portfolioValue) {
	//     err := executeStash(currentWallet, b.stashAmount, b.stashAddress)
	//     if err != nil {
	//         b.logMessage(fmt.Sprintf("Error during stash: %v", err))
	//     } else {
	//         b.logMessage("Stash executed successfully.")
	//     }
	// }

	b.logMessage("Portfolio check completed.")
}

func (b *CalypsoBot) logMessage(message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logEntry := fmt.Sprintf("[%s] %s\n", timestamp, message)
	b.log.SetText(b.log.Text + logEntry)
}