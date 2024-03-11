package main

import (
	"encoding/binary"
	"errors"

	"github.com/tendermint/tendermint/crypto/ed25519"

	"go.vocdoni.io/dvote/db"
)

type Account struct {
	Address       []byte
	Data          []byte
	State         []byte
	schnorrPubKey []byte
	//blsPubKey     []byte
	//pop           []byte
	counter  []byte
	Counter  uint32
	Amount   uint32
	Modified bool
	isNew    bool
}

func (app *App) fetchAccount(address []byte) (*Account, error) {
	account := &Account{}

	//maybe a previous tx has been delivered but not yet written to db
	//check the map of temp accounts for a changed amount
	logs.log("searching account in temporary memory...")

	var key [4]byte
	copy(key[:], address)

	v, ok := app.tempAccountMap[key]
	if ok {
		account = v
		return account, nil
	}

	//read from db
	logs.log("fetching account from internal database...")

	_, value, err := app.accountTree.Get(address)
	if err != nil {
		return nil, errors.New("Miss accountTree entry")
	}

	if len(value) < 88 {
		return nil, errors.New("Not enough data, some error occurred")
	}

	//fill values
	account.Data = value[:]
	account.Address = address
	account.schnorrPubKey = value[4:36]
	account.Amount = binary.BigEndian.Uint32(account.Data[:4])
	account.fetchCounter()
	account.fetchState()

	app.tempAccountMap[key] = account
	logs.logAccount(account)

	return account, nil
}

func (account *Account) fetchCounter() {
	if len(account.Data) < 88 {
		account.Counter = 0
	} else {
		account.counter = account.Data[84:88]
		account.Counter = binary.BigEndian.Uint32(account.counter)
	}
}

func (account *Account) fetchState() {
	if len(account.Data) == 120 {
		account.State = account.Data[88:]
	}
}

func (app *App) commitAccountsToDb() {
	logs.log("Commiting accounts to db... ")

	accBatch := db.NewBatch(app.accountLedgerDb)
	wAc := app.accountDb.WriteTx()

	for _, account := range app.tempAccountMap {
		if account.Modified {
			account.commitAccountToDb(app, accBatch, wAc)
		}
	}

	for _, account := range app.tempNewAccountMap {
		if account.Modified {
			account.commitNewAccountToDb(app, accBatch, wAc)
		}
	}

	//write deliver txs results on db
	wAc.Commit()
	wAc.Discard()
	accBatch.Commit()
	accBatch.Discard()

	//reset account cache
	app.tempAccountMap = make(map[[4]byte]*Account)
	app.tempNewAccountMap = make(map[[4]byte]*Account)
}

func (account *Account) commitAccountToDb(app *App, accBatch *db.Batch, wAc db.WriteTx) {
	//add to the stream to be commited to db
	err := app.accountTree.UpdateWithTx(wAc, account.Address, account.Data)
	if err != nil {
		logs.logError("Acctree update FATAL ERROR!!!", err)
		panic(err)
	}

	// reset the Modified flag
	account.Modified = false
}

func (account *Account) commitNewAccountToDb(app *App, accBatch *db.Batch, wAc db.WriteTx) {

	//add to the stream to be commited to db
	err1 := app.accountTree.AddWithTx(wAc, account.Address, account.Data)
	if err1 != nil {
		logs.logError("Acctree update FATAL ERROR!!!", err1)
	}

	// reset the Modified flag
	account.Modified = false

	//app.accountTree.PrintGraphviz(nil)

	//the following section creates a db with the bls pubkeys as keys
	//and account addresses as values (only if account watch is set)
	if !app.accountWatch {
		return
	}
	if len(account.Data) < 84 {
		return
	}
	err := accBatch.Set(account.Data[36:84], account.Address)
	if err != nil {
		logs.logError("ACCOUNT DB WRITE ERROR!!!", err)
	}
}

func (app *App) createTemplateAccount(seed []byte, amount uint32) {
	//produce keypair
	privkey := ed25519.GenPrivKeyFromSecret(seed)
	pubkey := privkey.PubKey()

	//create and fund account
	account := new(Account)
	account.isNew = true
	account.Amount = amount

	//find next account address
	account.nextAccountAddr(app)

	//create raw account data
	pad := make([]byte, 64)
	account.Data = append(pad[0:4], pubkey.Bytes()...)
	account.Data = append(account.Data, pad...)

	//write to Dbs
	account.writeAccount(app)
}

func (account *Account) writeAccount(app *App) {
	logs.log("Writting...  ")

	//create new array to avoid writing on slice coming from tx
	newData := make([]byte, 120)

	//fill the array with values
	binary.BigEndian.PutUint32(newData[:4], account.Amount)
	copy(newData[4:84], account.Data[4:84])
	binary.BigEndian.PutUint32(newData[84:88], account.Counter)
	account.Data = append(newData[:88], account.State...)

	//update temp account cache
	var key [4]byte
	copy(key[:], account.Address)
	account.Modified = true
	if account.isNew {
		app.tempNewAccountMap[key] = account
	} else {
		app.tempAccountMap[key] = account
	}

	logs.logAccount(account)
}

func (app *App) findAccountByPubKey(blskey []byte) (*Account, error) {
	rTx := app.accountLedgerDb.ReadTx()
	address, err := rTx.Get(blskey)
	if err != nil {
		logs.log("Failed to get address by bls key: ")
		return nil, err
	}

	account, err := app.fetchAccount(address)
	if err != nil {
		logs.logError("Account can't be found: ", err)
		return nil, err
	}
	logs.log("Found account:")
	logs.logAccount(account)

	return account, nil
}

func (account *Account) nextAccountAddr(app *App) {
	//find next account address
	nextaddr := app.accountNumOnDb

	account.Address = make([]byte, 4)
	binary.BigEndian.PutUint32(account.Address, uint32(nextaddr))
	app.accountNumOnDb++
}
