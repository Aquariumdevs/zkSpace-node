package main

import (
	"bytes"
	"encoding/binary"

	"github.com/tendermint/tendermint/crypto/ed25519"
)

func (tx *Transaction) isSigned(app *App) (code bool) {
	//load public key from account database
	logs.log("is it signed?")
	account, err := app.fetchAccount(tx.source)
	if err != nil {
		logs.logError("Could not fetch account to verify signature!!!", err)
		return false
	}
	tx.pubkey = account.schnorrPubKey
	pubkey := ed25519.PubKey(tx.pubkey)

	//parse message data
	tx.counter = account.counter
	hash := app.poseidon(append(tx.hash[:], account.counter...))

	//signature verification
	if !pubkey.VerifySignature(hash[:], tx.signature) {
		logs.log("Bad signature")
		logs.logTx(tx)
		return false
	}

	return true
}

func (tx *Transaction) selectTxType() (code bool) {
	logs.log("Type?")
	//tx.source = tx.data[:4]	//the same on all occasions

	//transaction format and type defined by the tx blob size

	switch tx.length {
	case 100:
		tx.isUpdate = true
		logs.log("	Update state")
		tx.state = tx.data[4:]
		return true

	case 68: //releases funds from staking (or delegation)
		tx.isRelease = true
		logs.log("	Release from staking")
		return true

	case 72:
		tx.isStake = true
		logs.log("	Stake")
		tx.amount = tx.data[4:8]
		return true

	case 74:
		tx.isDelegate = true
		logs.log("	Delegate")
		tx.amount = tx.data[8:10]
		tx.target = tx.data[4:8]
		return true

	case 76:
		tx.isTransfer = true
		logs.log("	Transfer simple")
		tx.target = tx.data[4:8]
		tx.amount = tx.data[8:12]
		return true

	case 108:
		tx.isTransfer = true
		logs.log("	Transfer with state update")
		tx.target = tx.data[4:8]
		tx.amount = tx.data[8:12]
		tx.state = tx.data[12:]
		return true

	case 244: // tx change account keys
		tx.isAccountKeyChanger = true
		logs.log("	Change Account keys")
		tx.target = nil
		tx.amount = nil
		tx.publickeys = tx.data[4:]
		return true

	case 248: // tx create account
		tx.isAccountCreator = true
		logs.log("	Create account")
		tx.target = nil
		tx.amount = tx.data[4:8]
		tx.publickeys = tx.data[8:]
		return true

	default:
		if tx.length < 73 {
			return false
		}

		//batch or contract
		tx.amount = tx.data[4:8]
		tx.pad = tx.data[8]
		if uint8(tx.pad) > 1 {
			tx.isBatch = true
			logs.log("	Batch transaction")
		} else {
			tx.isContract = true
			logs.log("	Contract")
			tx.target = tx.data[9:13]
			tx.payload = tx.data[13:]
		}
		return true
	}
	return false
}

func (tx *Transaction) verify() bool {
	return true
}

func (tx *Transaction) verifyBatch(app *App) bool {
	logs.log("verifying batch")

	//check that height recorded into the state is not larger than the current height
	maxheight := binary.BigEndian.Uint64(tx.state[32:40])
	if app.blockHeight > int64(maxheight) {
		return false
	}

	//parse participating  account addresses
	tx.addresses = tx.data[113:]

	//collect public keys
	var cpKeys [][]byte

	arraySize := len(tx.addresses) - 4
	for i := 0; i < arraySize; i += 4 {
		address := tx.addresses[i : i+4]
		account, err := app.fetchAccount(address)
		if err != nil {
			logs.logError("Problem with a batch entry: ", err)
			continue
		}
		blsPubKey := account.Data[36:84]
		cpKeys = append(cpKeys, blsPubKey)
	}

	// uncompress the signature and public keys
	sig := app.dummySig.Uncompress(tx.multisignature)
	PKeys := app.dummyPk.BatchUncompress(cpKeys)

	//verify aggregate signature
	var dst = []byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_")
	return sig.FastAggregateVerify(false, PKeys, tx.state, dst)
}

func (tx *Transaction) inCache(app *App) bool {
	logs.log("Checking cache for tx...")

	// Check the map first
	value, found := app.txMap[tx.hash]
	if found == false {
		return false
	}

	if bytes.Equal(tx.data, value.data) {
		logs.log("Already in cache!")
		return true
	}

	return false
}

func (tx *Transaction) verifyAccounts(app *App) (code uint32) {
	logs.log("Has valid accounts?")

	logs.log("Source account: ")
	account, err := app.fetchAccount(tx.source)
	if err != nil {
		logs.logError("source account not found: ", err)
		return 17
	}

	logs.log("Target account: ")
	if tx.target != nil {
		_, err = app.fetchAccount(tx.target)
	}

	if err != nil {
		logs.logError("target account not found: ", err)
		return 18
	}

	if tx.amount != nil {
		tx.Amount = binary.BigEndian.Uint32(tx.amount)
	}

	return tx.verifyFee(account, app)
}

func (tx *Transaction) verifyFee(account *Account, app *App) (code uint32) {
	logs.log("Has enough amount to pay fees?")

	tx.Fee = app.gas * uint32(tx.length)
	if tx.Amount+tx.Fee > account.Amount {
		logs.log("NO!")
		return 23
	}
	return 0
}

func (tx *Transaction) fetchTx(rawtx []byte, app *App) (code uint32) {
	logs.log("Fetching tx...")
	// check format
	tx.length = len(rawtx)

	//tx max size check
	if tx.length > 1401 {
		logs.log("Too big!")
		return 1
	}

	// tx min size check
	if tx.length < 68 {
		logs.log("Too small!")
		return 2
	}

	//parse values
	tx.signature = rawtx[:64]
	tx.data = rawtx[64:]
	tx.source = tx.data[:4]
	hash := app.poseidon(tx.data)
	copy(tx.hash[:], hash)

	return 0
}

func (tx *Transaction) isValid(app *App) (code uint32) {
	logs.log("Is valid?")
	if tx.inCache(app) {
		return 33
	}

	if !tx.isSigned(app) {
		return 39
	}

	if !tx.selectTxType() {
		return 44
	}

	if !tx.verifyBlsTx(app) {
		return 89
	}

	return tx.verifyAccounts(app)

	return 11
}
