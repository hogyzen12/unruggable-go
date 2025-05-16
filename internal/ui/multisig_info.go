package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gagliardetto/solana-go"
	"github.com/hogyzen12/squads-go/pkg/multisig"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func NewMultisigInfoScreen(win fyne.Window) fyne.CanvasObject {
	// ----------------------------------------------------------------
	// inputs
	// ----------------------------------------------------------------
	rpcEntry := widget.NewEntry()
	rpcEntry.SetText(GetGlobalState().RPCURL)

	addrEntry := widget.NewEntry()
	addrEntry.SetPlaceHolder("Multisig address (Base58)")

	// ----------------------------------------------------------------
	// output + action
	// ----------------------------------------------------------------
	output := widget.NewMultiLineEntry()
	output.Disable()

	fetchBtn := widget.NewButton("Fetch info", nil)

	fetchBtn.OnTapped = func() {
		addr, err := solana.PublicKeyFromBase58(strings.TrimSpace(addrEntry.Text))
		if err != nil {
			dialog.ShowError(err, win)
			return
		}

		fetchBtn.Disable()
		output.SetText("Loading…")

		go func() {
			info, err := multisig.FetchMultisigInfo(
				context.Background(), rpcEntry.Text, addr)

			// UI update ------------------------------------------------
			if err != nil {
				output.SetText("Error: " + err.Error())
			} else {
				var b strings.Builder
				b.WriteString("═══════════════════════════════════\n")
				b.WriteString("        MULTISIG INFORMATION       \n")
				b.WriteString("═══════════════════════════════════\n\n")
				b.WriteString("Address:   " + info.Address.String() + "\n")
				b.WriteString("Threshold: " + strconv.Itoa(int(info.Threshold)) + "\n")
				b.WriteString("Timelock:  " + strconv.Itoa(int(info.TimeLock)) + " sec\n\n")

				b.WriteString("Members (" + strconv.Itoa(len(info.Members)) + "):\n")
				for i, m := range info.Members {
					b.WriteString(fmt.Sprintf("  %2d. %s  (mask=%d)\n",
						i+1, m.Key.String(), m.Permissions.Mask))
				}
				b.WriteString("\nDefault vault (index 0):\n  " + info.DefaultVault.String() + "\n")
				b.WriteString(fmt.Sprintf("\nTransaction index:      %d\n", info.TransactionIndex))
				b.WriteString(fmt.Sprintf("Stale transaction index: %d\n", info.StaleTransactionIndex))

				output.SetText(b.String())
			}

			fetchBtn.Enable()
			win.Canvas().Refresh(output)
		}()
	}

	// ----------------------------------------------------------------
	// layout
	// ----------------------------------------------------------------
	form := widget.NewForm(
		widget.NewFormItem("RPC endpoint", rpcEntry),
		widget.NewFormItem("Multisig address", addrEntry),
	)

	return container.NewVBox(form, fetchBtn, output)
}
