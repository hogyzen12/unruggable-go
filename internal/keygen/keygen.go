package keygen

import (
	"fmt"
	"github.com/taurusgroup/frost-ed25519/pkg/eddsa"
	"github.com/taurusgroup/frost-ed25519/pkg/frost"
	"github.com/taurusgroup/frost-ed25519/pkg/frost/keygen"
	"github.com/taurusgroup/frost-ed25519/pkg/frost/party"
	"github.com/taurusgroup/frost-ed25519/pkg/helpers"
	"github.com/taurusgroup/frost-ed25519/pkg/state"
)

const MaxN = 100

// GenerateKeys function generates the MPC keys based on t and n values
func GenerateKeys(t, n int) string {
	partyIDs := helpers.GenerateSet(party.ID(n))

	// structure holding parties' state and output
	states := map[party.ID]*state.State{}
	outputs := map[party.ID]*keygen.Output{}

	// create a state for each party
	for _, id := range partyIDs {
		var err error
		states[id], outputs[id], err = frost.NewKeygenState(id, partyIDs, party.Size(t), 0)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
	}

	msgsOut1 := make([][]byte, 0, n)
	msgsOut2 := make([][]byte, 0, n*(n-1)/2)

	for _, s := range states {
		msgs1, err := helpers.PartyRoutine(nil, s)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		msgsOut1 = append(msgsOut1, msgs1...)
	}

	for _, s := range states {
		msgs2, err := helpers.PartyRoutine(msgsOut1, s)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		msgsOut2 = append(msgsOut2, msgs2...)
	}

	for _, s := range states {
		_, err := helpers.PartyRoutine(msgsOut2, s)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
	}

	// Get the public data
	result := "Group Key:\n"
	id0 := partyIDs[0]
	if err := states[id0].WaitForError(); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	public := outputs[id0].Public
	secrets := make(map[party.ID]*eddsa.SecretShare, n)
	groupKey := public.GroupKey
	result += fmt.Sprintf("  %x\n\n", groupKey.ToEd25519())

	for _, id := range partyIDs {
		if err := states[id].WaitForError(); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		shareSecret := outputs[id].SecretKey
		sharePublic := public.Shares[id]
		secrets[id] = shareSecret
		result += fmt.Sprintf("Party %d:\n  secret: %x\n  public: %x\n", id, shareSecret.Secret.Bytes(), sharePublic.Bytes())
	}

	return result
}
