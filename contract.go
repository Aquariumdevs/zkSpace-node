package main

import (
	"encoding/binary"

	"go.vocdoni.io/dvote/db"
)

type Contract struct {
	Address []byte
	counter []byte
	Counter uint64
	Payload []byte
}

func (app *App) commitContractsToDb() {
	logs.log("Commiting contracts to db... ")

	app.ctxDbMutex.Lock()
	conBatch := db.NewBatch(app.contractStorageDb)
	conLedgBatch := db.NewBatch(app.contractLedgerDb)
	wCn := app.contractDb.WriteTx()

	//prepare old contracts for updating
	for _, contract := range app.tempContractMap {
		err := app.contractTree.UpdateWithTx(wCn, contract.Address, contract.counter)
		if err != nil {
			panic(err)
		}
		app.commitContractToDb(contract, conBatch)
	}

	//commit old contracts to tree
	wCn.Commit()
	wCn.Discard()

	//prepare new contracts for writing on tree and db
	zerocounter := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	var newContractKeys, newContractValues [][]byte

	for _, contract := range app.tempNewContractMap {
		newContractKeys = append(newContractKeys, contract.Address[:])
		newContractValues = append(newContractValues, zerocounter)
		contract.counter = zerocounter[:]
		app.commitContractToDb(contract, conBatch)
		app.commitContractToLedger(contract, conLedgBatch)
	}

	//add new contracts to contract tree
	app.contractTree.AddBatch(newContractKeys, newContractValues)

	//now write contract payloads to db for data availability
	conBatch.Commit()
	conBatch.Discard()

	conLedgBatch.Commit()
	conLedgBatch.Discard()

	app.ctxDbMutex.Unlock()

	//reset contract maps
	app.tempContractMap = make(map[[4]byte]*Contract)
	app.tempNewContractMap = make(map[[4]byte]*Contract)
}

func (app *App) commitContractToLedger(contract *Contract, conLedgBatch *db.Batch) {
	logs.log("Commiting contract to ledger... ")

	if !app.accountWatch {
		return
	}

	hash := app.sha2(contract.Payload)
	err := conLedgBatch.Set(hash, contract.Address)
	if err != nil {
		panic(err)
	}
}

func (app *App) commitContractToDb(contract *Contract, conBatch *db.Batch) {
	logs.log("Commiting contract to db... ")

	address := append(contract.Address, contract.counter...)
	err := conBatch.Set(address, contract.Payload)
	if err != nil {
		panic(err)
	}
}

func (contract *Contract) createContract(app *App, key [4]byte) {
	logs.log("Creating contract... ")

	var nextaddr [4]byte
	nextAddr := uint32(app.contractNumOnDb + len(app.tempNewContractMap))
	binary.BigEndian.PutUint32(nextaddr[:], nextAddr)
	contract.Address = nextaddr[:]
	contract.counter = []byte{0, 0, 0, 0, 0, 0, 0, 0}
	app.tempNewContractMap[nextaddr] = contract
}

func (contract *Contract) writeContract(app *App, key [4]byte) {
	logs.log("Writting contract to temporary map... ")

	binary.BigEndian.PutUint64(contract.counter, contract.Counter)
	app.tempContractMap[key] = contract
}

func (app *App) fetchContract(key [4]byte) *Contract {
	logs.log("Fetching contract... ")

	contract := &Contract{}

	//look at the map first
	v, ok := app.tempContractMap[key]
	if ok {
		contract = v
		return v
	}

	_, counter, err := app.contractTree.Get(key[:])
	if err != nil {
		return contract
	}

	contract.counter = counter
	contract.Counter = binary.BigEndian.Uint64(counter)
	contract.Address = key[:]
	app.tempContractMap[key] = contract

	return contract
}

func (app *App) findContractBypHash(pHash []byte) (*Contract, error) {
	logs.log("Searching contract in db by pHash... ")

	rTx := app.accountLedgerDb.ReadTx()
	var address [4]byte

	addr, err := rTx.Get(pHash)
	if err != nil {
		logs.logError("Contract cannot be not found: ", err)
		return nil, err
	}

	copy(address[:], addr)

	contract := app.fetchContract(address)

	return contract, nil
}
