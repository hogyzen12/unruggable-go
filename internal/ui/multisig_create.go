package ui

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"unruggable-go/internal/storage"

	"github.com/gagliardetto/solana-go"
	"github.com/hogyzen12/squads-go/pkg/multisig"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type memberRow struct {
	keyEntry   *widget.Entry
	permSelect *widget.Select
}

const (
	permFull    = "Full (7)"
	permVote    = "Vote (2)"
	permPropose = "Propose (1)"
	permExecute = "Execute (4)"
	permCustom  = "Custom…"
)

func permMask(label string) uint8 {
	switch label {
	case permFull:
		return 7
	case permVote:
		return 2
	case permPropose:
		return 1
	case permExecute:
		return 4
	default:
		return 0
	}
}

func NewMultisigCreateScreen(win fyne.Window) fyne.CanvasObject {
	//----------------------------------------------------------------
	// RPC + admin
	//----------------------------------------------------------------
	rpcEntry := widget.NewEntry()
	rpcEntry.SetText(GetGlobalState().RPCURL)

	adminEntry := widget.NewEntry() // auto-filled after wallet unlock
	adminEntry.Disable()

	status := widget.NewLabel("")

	//----------------------------------------------------------------
	// members table widgets must be declared before helper funcs
	//----------------------------------------------------------------
	rowsBox := container.NewVBox()
	var rows []*memberRow

	addRow := func() {
		e := widget.NewEntry()
		s := widget.NewSelect(
			[]string{permFull, permVote, permPropose, permExecute, permCustom},
			nil,
		)
		s.SetSelected(permFull)
		row := &memberRow{keyEntry: e, permSelect: s}
		rows = append(rows, row)
		rowsBox.Add(container.NewGridWithColumns(2, e, s))
	}

	addRow() // initial row

	//----------------------------------------------------------------
	// threshold slider
	//----------------------------------------------------------------
	thresholdSlider := widget.NewSlider(1, 1)
	thresholdSlider.Step = 1
	thresholdLabel := widget.NewLabel("Threshold: 1")

	updateSlider := func() {
		voters := 0
		for _, r := range rows {
			if permMask(r.permSelect.Selected)&multisig.PermissionVote != 0 {
				voters++
			}
		}
		if voters == 0 {
			voters = 1
		}
		thresholdSlider.Max = float64(voters)
		if thresholdSlider.Value > thresholdSlider.Max {
			thresholdSlider.SetValue(thresholdSlider.Max)
		}
		thresholdLabel.SetText("Threshold: " + strconv.Itoa(int(thresholdSlider.Value)))
		thresholdSlider.Refresh()
	}
	thresholdSlider.OnChanged = func(v float64) {
		thresholdLabel.SetText("Threshold: " + strconv.Itoa(int(v)))
	}

	//----------------------------------------------------------------
	// action buttons
	//----------------------------------------------------------------
	createBtn := widget.NewButton("Create multisig", nil)
	createBtn.Disable() // enabled after wallet unlock

	//----------------------------------------------------------------
	// keep admin private key around after unlock
	//----------------------------------------------------------------
	var adminPriv solana.PrivateKey

	unlockBtn := widget.NewButtonWithIcon("Use selected wallet", theme.LoginIcon(), func() {
		wid := GetGlobalState().GetSelectedWallet()
		if wid == "" {
			dialog.ShowInformation("No wallet", "Select a wallet first", win)
			return
		}

		pass := widget.NewPasswordEntry()
		dialog.ShowCustomConfirm("Unlock wallet", "Unlock", "Cancel", pass, func(ok bool) {
			if !ok {
				return
			}
			store := storage.NewWalletStorage(fyne.CurrentApp())
			wmap, err := store.LoadWallets()
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			enc, ok := wmap[wid]
			if !ok {
				dialog.ShowError(errors.New("wallet not found"), win)
				return
			}
			dec, err := decrypt(enc, pass.Text)
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			adminPriv = solana.MustPrivateKeyFromBase58(string(dec))
			adminEntry.SetText(adminPriv.PublicKey().String())
			adminEntry.Refresh()
			createBtn.Enable()
		}, win)
	})

	//----------------------------------------------------------------
	// link permission selects to slider update
	//----------------------------------------------------------------
	for _, r := range rows {
		r := r
		r.permSelect.OnChanged = func(string) { updateSlider() }
	}

	addMemberBtn := widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
		addRow()
		// hook new row’s select
		rows[len(rows)-1].permSelect.OnChanged = func(string) { updateSlider() }
		updateSlider()
	})

	//----------------------------------------------------------------
	// create multisig handler
	//----------------------------------------------------------------
	createBtn.OnTapped = func() {
		if adminPriv == nil {
			dialog.ShowError(errors.New("wallet not unlocked"), win)
			return
		}

		var members []multisig.Member
		for _, r := range rows {
			for _, k := range strings.Split(r.keyEntry.Text, ",") {
				k = strings.TrimSpace(k)
				if k == "" {
					continue
				}
				pk, err := solana.PublicKeyFromBase58(k)
				if err != nil {
					dialog.ShowError(err, win)
					return
				}
				members = append(members, multisig.Member{
					Key:         pk,
					Permissions: permMask(r.permSelect.Selected),
				})
			}
		}

		if len(members) == 0 {
			dialog.ShowError(errors.New("add at least one member"), win)
			return
		}

		createBtn.Disable()
		status.SetText("Submitting…")

		go func() {
			sig, addr, _, err := multisig.CreateMultisigWithParams(context.Background(),
				multisig.CreateParams{
					RPCURL:    rpcEntry.Text,
					WSURL:     strings.Replace(rpcEntry.Text, "https://", "wss://", 1),
					Payer:     adminPriv,
					Members:   members,
					Threshold: uint16(thresholdSlider.Value),
				})

			if err != nil {
				dialog.ShowError(err, win)
				status.SetText("Error: " + err.Error())
			} else {
				dialog.ShowInformation("Multisig created",
					"PDA:\n"+addr.String()+"\n\nTx:\n"+sig.String(), win)
				status.SetText("Multisig: " + addr.String())
			}
			createBtn.Enable()
			win.Canvas().Refresh(status)
		}()
	}

	//----------------------------------------------------------------
	// assemble UI
	//----------------------------------------------------------------
	form := container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("RPC endpoint", rpcEntry),
			widget.NewFormItem("Admin key", adminEntry),
		),
		unlockBtn,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Members", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		rowsBox,
		addMemberBtn,
		widget.NewSeparator(),
		thresholdLabel,
		thresholdSlider,
		createBtn,
		status,
	)

	scroll := container.NewVScroll(container.NewPadded(form))
	scroll.SetMinSize(fyne.NewSize(520, 480))
	return scroll
}
