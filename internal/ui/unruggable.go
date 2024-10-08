package ui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

type UnruggableScreen struct {
	container      *fyne.Container
	sessionIDEntry *widget.Entry
	tEntry         *widget.Entry
	nEntry         *widget.Entry
	statusLabel    *widget.Label
	logDisplay     *widget.Entry
	apiURL         string
	window         fyne.Window
	app            fyne.App
	sharesList     *widget.List
}

func (u *UnruggableScreen) refreshSharesDisplay() {
	shares, _ := GetSavedShares(u.app)
	u.sharesList.Refresh()

	// Calculate the height based on the number of shares
	itemHeight := 30 // Estimate the height of each item
	desiredHeight := len(shares) * itemHeight
	maxHeight := 300 // Maximum height for the list

	// Set the minimum size of the sharesList
	if desiredHeight > maxHeight {
		desiredHeight = maxHeight
	}
	u.sharesList.MinSize()
	u.sharesList.Resize(fyne.NewSize(u.sharesList.Size().Width, float32(desiredHeight)))
}

func NewUnruggableScreen(window fyne.Window, app fyne.App) fyne.CanvasObject {
	u := &UnruggableScreen{
		sessionIDEntry: widget.NewEntry(),
		tEntry:         widget.NewEntry(),
		nEntry:         widget.NewEntry(),
		statusLabel:    widget.NewLabel(""),
		logDisplay:     widget.NewMultiLineEntry(),
		apiURL:         "https://frost-api-small-snowflake-9992.fly.dev",
		window:         window,
		app:            app,
	}

	u.logDisplay.Disable()
	u.logDisplay.SetMinRowsVisible(9)

	// Layout for threshold and total parties
	thresholdPartyForm := container.NewGridWithColumns(4,
		widget.NewLabel("Threshold (t):"),
		u.tEntry,
		widget.NewLabel("Total Parties (n):"),
		u.nEntry,
	)

	initButton := widget.NewButton("Initialize Session", u.initializeSession)

	sessionIDForm := container.NewBorder(nil, nil, widget.NewLabel("Session ID:"), nil, u.sessionIDEntry)
	joinButton := widget.NewButton("Join Session", u.joinSession)

	u.sharesList = widget.NewList(
		func() int {
			shares, _ := GetSavedShares(u.app)
			return len(shares)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("Template")
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			shares, _ := GetSavedShares(u.app)
			if id < len(shares) {
				share := shares[id]
				label := item.(*widget.Label)
				label.SetText(fmt.Sprintf("Group Key: %s... | Party ID: %d | Secret Share: %s... | Public Share: %s...",
					share.GroupKey[:8], share.PartyID, share.SecretShare[:8], share.PublicShare[:8]))
			}
		},
	)
	// Set a minimum size for the sharesList to show more rows
	u.sharesList.MinSize()

	// Create a container for the sharesList with a fixed height
	//sharesContainer := container.NewStack(u.sharesList)

	u.container = container.NewVBox(
		widget.NewLabel("Unruggable MPC Setup"),
		thresholdPartyForm,
		initButton,
		sessionIDForm,
		joinButton,
		u.statusLabel,
		widget.NewLabel("Saved Shares:"),
		u.sharesList,
		widget.NewLabel("Log:"),
		u.logDisplay,
	)

	// Set up a listener to refresh the shares display when the window is resized
	window.Canvas().SetOnTypedKey(func(ke *fyne.KeyEvent) {
		u.refreshSharesDisplay()
	})

	u.refreshSharesDisplay()

	return u.container
}

func (u *UnruggableScreen) initializeSession() {
	t, err := strconv.Atoi(u.tEntry.Text)
	if err != nil {
		dialog.ShowError(fmt.Errorf("Invalid threshold value"), fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}

	n, err := strconv.Atoi(u.nEntry.Text)
	if err != nil {
		dialog.ShowError(fmt.Errorf("Invalid total parties value"), fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}

	u.appendLog(fmt.Sprintf("Initializing session with t=%d and n=%d", t, n))
	sessionID, err := initiateSession(u.apiURL, t, n)
	if err != nil {
		u.appendLog(fmt.Sprintf("Error initializing session: %v", err))
		dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}

	u.sessionIDEntry.SetText(sessionID)
	u.statusLabel.SetText(fmt.Sprintf("Session initialized: %s", sessionID))
	u.appendLog(fmt.Sprintf("Session initialized with ID: %s", sessionID))
}

func (u *UnruggableScreen) joinSession() {
	sessionID := u.sessionIDEntry.Text
	if sessionID == "" {
		dialog.ShowError(fmt.Errorf("Please enter a session ID"), u.window)
		return
	}

	go func() {
		u.appendLog(fmt.Sprintf("Joining session: %s", sessionID))
		partyID, partyIDs, t, err := joinAndRetrieveSessionInfo(u.apiURL, sessionID, u.appendLog)
		if err != nil {
			u.appendLog(fmt.Sprintf("Error joining session: %v", err))
			dialog.ShowError(err, u.window)
			return
		}

		statusText := fmt.Sprintf("Joined session as party %d with parties %v and threshold %d", partyID, partyIDs, t)
		u.statusLabel.SetText(statusText)
		u.appendLog(statusText)

		// Perform key generation
		u.appendLog("Starting key generation process")
		err = performKeyGeneration(u.apiURL, sessionID, partyID, partyIDs, t, u.appendLog, u.window, u.app)
		if err != nil {
			u.appendLog(fmt.Sprintf("Error during key generation: %v", err))
			dialog.ShowError(err, u.window)
			return
		}

		u.statusLabel.SetText("Key generation completed successfully")
		u.appendLog("Key generation completed successfully")

		// Refresh the shares display after successful key generation
		u.refreshSharesDisplay()
	}()
}

func (u *UnruggableScreen) appendLog(message string) {
	u.logDisplay.SetText(u.logDisplay.Text + message + "\n")
	u.logDisplay.CursorRow = len(strings.Split(u.logDisplay.Text, "\n")) - 1
	u.logDisplay.Refresh()
}

// The following methods are not used in this implementation but are required to satisfy the fyne.CanvasObject interface
func (u *UnruggableScreen) Move(position fyne.Position) {
	u.container.Move(position)
}

func (u *UnruggableScreen) Resize(size fyne.Size) {
	u.container.Resize(size)
}

func (u *UnruggableScreen) Position() fyne.Position {
	return u.container.Position()
}

func (u *UnruggableScreen) Size() fyne.Size {
	return u.container.Size()
}

func (u *UnruggableScreen) MinSize() fyne.Size {
	return u.container.MinSize()
}

func (u *UnruggableScreen) Visible() bool {
	return u.container.Visible()
}

func (u *UnruggableScreen) Show() {
	u.container.Show()
}

func (u *UnruggableScreen) Hide() {
	u.container.Hide()
}

func (u *UnruggableScreen) Refresh() {
	u.container.Refresh()
}
