package main

import (
	"encoding/binary"

	abcitypes "github.com/tendermint/tendermint/abci/types"
)

func (tx *Transaction) execBatch(app *App) {
	logs.log("Executing Batch")

	//truncate maxheight from state
	tx.state = tx.state[:32]
	tx.target = tx.source

	length := len(tx.addresses)
	for i := 0; i+4 < length; i += 4 {
		tx.source = tx.addresses[i : i+4]
		tx.length = 0      //this leads to zero fees fees are paid from the batcer
		tx.execUpdate(app) //the amount will be subtracted from every participant
	}

	account, err := app.fetchAccount(tx.target)
	if err != nil {
		logs.logError("Failed to fetch account: ", err)
		panic(err)
	}
	account.Amount += tx.Amount*uint32(length/4) - tx.Fee
	app.totalFees += tx.Fee

	account.writeAccount(app)
}

func (tx *Transaction) execRelease(app *App) {
	logs.log("Executing release from staking")

	var val abcitypes.ValidatorUpdate
	val.Power = 0
	var err error

	account, err := app.fetchAccount(tx.source)
	if err != nil {
		//this should not happen
		logs.logError("source account not found: ", err)
		return
	}

	val.PubKey, err = app.toPk(account.schnorrPubKey)
	if err != nil {
		logs.logError("Failed to convert public key: ", err)
		panic(err)
	}

	addr := app.toAddress(account.schnorrPubKey)

	_, valAccount, err := app.validatorTree.Get(addr)
	if err != nil {
		logs.logError("Failed to get element from validator tree: ", err)
		return
	}

	app.valUpdates = append(app.valUpdates, val)
	Amount := binary.BigEndian.Uint64(valAccount[:8])
	tx.Amount = uint32(Amount)

	tx.execUpdate(app)

	binary.BigEndian.PutUint64(valAccount[:8], uint64(0))

	logs.log("Updated validator amount (binary) and valpower: ")
	logs.log(valAccount[:8])
	logs.log(val.Power)

	app.validatorTree.Update(addr, valAccount)
}

func (tx *Transaction) execStake(app *App) {
	logs.log("Executing stake")

	account := tx.execUpdate(app)
	var err error

	var val abcitypes.ValidatorUpdate
	val.Power = int64(tx.Amount)

	val.PubKey, err = app.toPk(account.schnorrPubKey)
	if err != nil {
		logs.logError("Failed to convert public key: ", err)
		panic(err)
	}

	addr := app.toAddress(account.schnorrPubKey)

	_, valAccount, err := app.validatorTree.Get(addr)
	if err == nil {
		val.Power += int64(binary.BigEndian.Uint64(valAccount[:8]))
	} else {
		logs.log("Validator not found, creating new...")
		valAccount = make([]byte, 40)
	}

	binary.BigEndian.PutUint64(valAccount[:8], uint64(val.Power))
	logs.log("Updated validator amount (binary) and power: ")
	logs.log(valAccount[:8])
	logs.log(val.Power)

	if err == nil {
		app.validatorTree.Update(addr, valAccount)
	} else {
		logs.log("Validator not found, adding new...")
		copy(valAccount[8:], account.schnorrPubKey)
		app.validatorTree.Add(addr, valAccount)
	}

	app.valUpdates = append(app.valUpdates, val)
}

func (tx *Transaction) execUpdate(app *App) *Account {
	logs.log("Executing state update")

	/// update source account
	// Fetch account
	account, err := app.fetchAccount(tx.source)
	if err != nil {
		//this should not happen
		logs.logError("source account not found: ", err)
		return nil
	}

	logs.log("Update source account: ")
	logs.logAccount(account)

	if tx.isRelease {
		// Add amount and subtract Fee from account
		account.Amount += (tx.Amount - tx.Fee)
	} else {
		// Subtract amount and Fee from account
		account.Amount -= (tx.Amount + tx.Fee)
	}
	app.totalFees += tx.Fee

	//Update counter on every tx
	account.Counter++

	//update state
	account.State = tx.state

	// Write updated account to database
	logs.log("Updated source account: ")
	account.writeAccount(app)

	return account
}

func (tx *Transaction) execTransfer(app *App) {
	logs.log("Executing transfer")

	// update target account
	// Fetch account
	tx.execUpdate(app)

	account, err := app.fetchAccount(tx.target)
	if err != nil {
		logs.logError("Target account not found!", err)
		return
	}

	logs.log("Update target account: ")
	logs.logAccount(account)

	// Subtract amount from account
	account.Amount += tx.Amount

	// Write updated account to database
	logs.log("Updated target account: ")
	account.writeAccount(app)
}

func (tx *Transaction) execAccountKeyChanger(app *App) {
	logs.log("Executing changing keys...")

	account, err := app.fetchAccount(tx.source)
	if err != nil {
		logs.logError("ExecKeyChanger failed because account not found: ", err)
	}
	//source and public key !!!attention
	account.Data = tx.data

	account.Address = tx.source

	//fees
	account.Amount -= tx.Fee
	app.totalFees += tx.Fee

	//Update counter on every tx
	account.Counter++

	account.writeAccount(app)
}

func (tx *Transaction) execCreateAccount(app *App) []byte {
	logs.log("Executing creating new account...")
	account := new(Account)
	account.isNew = true

	//amount and public key
	account.Data = tx.data[4:88]
	account.schnorrPubKey = account.Data[4:36]
	account.Amount = tx.Amount

	//find next account address
	account.nextAccountAddr(app)

	//create new account entry
	account.writeAccount(app)

	//tx.target = account.Address
	tx.execUpdate(app)

	return account.Address
}

func (tx *Transaction) execContract(app *App) []byte {
	logs.log("Executing contract")

	var key [4]byte
	copy(key[:], tx.target)

	Target := binary.BigEndian.Uint32(tx.target)

	tx.execUpdate(app)

	contract := app.fetchContract(key)

	contract.Payload = tx.payload
	///key insteadof counter???
	/////TODO: explanation, logs
	if Target >= uint32(app.contractNumOnDb) {
		contract.createContract(app, key)
	} else {
		contract.Counter++
		contract.writeContract(app, key)
	}
	data := append(tx.hash[:], contract.Address...)
	hash := app.sha2(data)
	copy(tx.hash[:], hash)

	return contract.Address
}
