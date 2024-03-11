// tools.go
package main

import (
	"crypto/sha256"
	//"kvstore/poseidon"

	"github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/crypto/encoding"
	"github.com/tendermint/tendermint/proto/tendermint/crypto"
	//"github.com/vocdoni/arbo"
)

/*
	func (app *App) poseidon(data []byte) []byte {
		h2, err := poseidon.HashBytes(data)
		if err != nil {
			logs.logError("POSEIDON ERROR________________________ _ _ _", err)
			panic(err)
		}

		result := arbo.BigIntToBytes(32, h2)
		return result
	}
*/
func (app *App) sha2(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	sha_2 := h.Sum(nil)
	return sha_2
}

func (app *App) toAddress(key []byte) []byte {
	keyhash := app.sha2(key)
	return keyhash[:20]
}

func (app *App) toPk(key []byte) (crypto.PublicKey, error) {
	pke := ed25519.PubKey(key)

	pkp, err := encoding.PubKeyToProto(pke)
	if err != nil {
		logs.logError("Failed to encode public key :", err)
		panic(err)
		return pkp, err
	}
	return pkp, nil
}

func (app *App) removeDuplicateValidatorUpdates() {
	seen := make(map[string]types.ValidatorUpdate)

	// Iterate through the updates and keep the rightmost occurrences
	for _, update := range app.valUpdates {
		key := update.PubKey.String()
		seen[key] = update
	}

	// delete the updates slice
	app.valUpdates = make([]types.ValidatorUpdate, 0)

	// Refill the slice from the map values
	for _, update := range seen {
		app.valUpdates = append(app.valUpdates, update)
	}
}
