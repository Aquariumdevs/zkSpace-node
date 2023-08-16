package main

import (
	"bytes"
	//"errors"
	//"fmt"
	"sync"

	abcitypes "github.com/tendermint/tendermint/abci/types"

	"github.com/tendermint/tendermint/crypto/encoding"

	"encoding/binary"

	"github.com/vocdoni/arbo"
	"go.vocdoni.io/dvote/db"
	badb "go.vocdoni.io/dvote/db/badgerdb"
)

type App struct {
	//databases and trees
	accountLedgerDb  *badb.BadgerDB
	contractLedgerDb *badb.BadgerDB

	contractStorageDb  *badb.BadgerDB
	contractStorageDb2 *badb.BadgerDB

	accountDb    *badb.BadgerDB
	contractDb   *badb.BadgerDB
	txStorageDb  *badb.BadgerDB
	txStorageDb2 *badb.BadgerDB
	blockHashDb  *badb.BadgerDB
	validatorDb  *badb.BadgerDB

	accountTree    *arbo.Tree
	contractTree   *arbo.Tree
	txStorageTree  *arbo.Tree
	txStorageTree2 *arbo.Tree
	blockHashTree  *arbo.Tree
	validatorTree  *arbo.Tree

	//transaction cache
	txMap map[[32]byte]*Transaction

	//account and contract cache
	tempAccountMap     map[[4]byte]*Account
	tempNewAccountMap  map[[4]byte]*Account
	tempContractMap    map[[4]byte]*Contract
	tempNewContractMap map[[4]byte]*Contract

	//set this to build a temporary database for querrying addresses with bls keys
	accountWatch bool

	log Logger

	blockheight [8]byte
	blockHeight int64

	prevHash []byte

	txDbMutex  sync.Mutex
	ctxDbMutex sync.Mutex

	//Validator updates
	valUpdates []abcitypes.ValidatorUpdate

	//transaction db entries as batch
	txDbKeys, txDbVals [][]byte

	//current size of DBs
	accountNumOnDb  int
	contractNumOnDb int

	//dummy curve points for bls
	dummySig *Signature
	dummyPk  *PublicKey

	//financial parameters
	emptyVoteLeak int64
	gas           uint32
	blockReward   int64

	totalFees uint32
}

func NewApp() (*App, error) {
	app := &App{}

	// create badger databases and associated arbo merkle trees
	accountLedgerDb, err := badb.New(db.Options{Path: "accdb"})
	if err != nil {
		logs.logError("Account db can not be created: ", err)
		return nil, err
	}

	contractLedgerDb, err := badb.New(db.Options{Path: "condb"})
	if err != nil {
		logs.logError("Contract db can not be created: ", err)
		return nil, err
	}

	// create 2 temporary swaping databases of contract entries
	contractStorageDb, err := badb.New(db.Options{Path: contractdb})
	if err != nil {
		logs.logError("Contract storage db can not be created: ", err)
		return nil, err
	}

	contractStorageDb2, err := badb.New(db.Options{Path: contractdb2})
	if err != nil {
		logs.logError("Contract storage db2 can not be created: ", err)
		return nil, err
	}

	// create new Tree of accounts with maxLevels=48 and Blake2b hash function
	accountDb, accountTree, err := app.createTreeDb("badg", 48, false)
	if err != nil {
		logs.logError("accountTree ledger failed!!!", err)
		return nil, err
	}

	// create Tree of contracts
	contractDb, contractTree, err := app.createTreeDb(badg0, 48, false)
	if err != nil {
		logs.logError("contractTree ledger failed!!!", err)
	}

	// create 2 temporary swaping Trees of transactions

	txStorageDb, txStorageTree, err := app.createTreeDb(badg2, 64, true)
	if err != nil {
		// handle error
		logs.logError("Tree txStorageTree failed!!!", err)
		return nil, err
	}
	txStorageDb2, txStorageTree2, err := app.createTreeDb(badg3, 64, true)
	if err != nil {
		logs.logError("Tree txStorageTree failed!!!", err)
		return nil, err
	}

	//create a tree of blockhashes
	blockHashDb, blockHashTree, err := app.createTreeDb("badg4", 64, true)
	if err != nil {
		logs.logError("Tree blockHashTree failed!!!", err)
		return nil, err
	}

	validatorDb, validatorTree, err := app.createTreeDb("badg5", 256, false)
	if err != nil {
		logs.logError("Validafor Tree failed", err)
	}

	//initialize maps for temporary storage and fast access for transactions and accounts
	txMap := make(map[[32]byte]*Transaction)
	tempAccountMap := make(map[[4]byte]*Account)
	tempNewAccountMap := make(map[[4]byte]*Account)
	tempContractMap := make(map[[4]byte]*Contract)
	tempNewContractMap := make(map[[4]byte]*Contract)

	//constructing the app
	app = &App{
		//initialize parameters
		gas:           uint32(100),
		emptyVoteLeak: int64(1),
		blockReward:   int64(100000),
		accountWatch:  true, // toggle for watching accounts (register a db with bls public keys as db keys

		//parse databases and trees
		accountLedgerDb:    accountLedgerDb,
		contractLedgerDb:   contractLedgerDb,
		contractStorageDb:  contractStorageDb,
		contractStorageDb2: contractStorageDb2,
		contractDb:         contractDb,
		accountDb:          accountDb,
		txStorageDb:        txStorageDb,
		txStorageDb2:       txStorageDb2,
		blockHashDb:        blockHashDb,
		validatorDb:        validatorDb,
		accountTree:        accountTree,
		contractTree:       contractTree,
		txStorageTree:      txStorageTree,
		txStorageTree2:     txStorageTree2,
		blockHashTree:      blockHashTree,
		validatorTree:      validatorTree,

		//parse maps
		txMap:              txMap,
		tempAccountMap:     tempAccountMap,
		tempNewAccountMap:  tempNewAccountMap,
		tempContractMap:    tempContractMap,
		tempNewContractMap: tempNewContractMap,
	}

	app.dummySig = new(Signature)
	app.dummyPk = new(PublicKey)

	//log everything
	logs.debugLogs = true

	//example accounts creation

	app.accountNumOnDb = 0

	//first example account creation
	app.createTemplateAccount([]byte("Iloveyou!"), 50000000)

	//second example account creation
	app.createTemplateAccount([]byte("Iloveher"), 500000)

	app.commitAccountsToDb()

	return app, nil
}

var _ abcitypes.Application = (*App)(nil)

func (App) Info(req abcitypes.RequestInfo) abcitypes.ResponseInfo {
	return abcitypes.ResponseInfo{}
}

func (App) SetOption(req abcitypes.RequestSetOption) abcitypes.ResponseSetOption {
	return abcitypes.ResponseSetOption{}
}

func (App) OfferSnapshot(req abcitypes.RequestOfferSnapshot) abcitypes.ResponseOfferSnapshot {
	return abcitypes.ResponseOfferSnapshot{}
}

func (App) LoadSnapshotChunk(req abcitypes.RequestLoadSnapshotChunk) abcitypes.ResponseLoadSnapshotChunk {
	return abcitypes.ResponseLoadSnapshotChunk{}
}

func (App) ListSnapshots(req abcitypes.RequestListSnapshots) abcitypes.ResponseListSnapshots {
	return abcitypes.ResponseListSnapshots{}
}

func (app *App) InitChain(req abcitypes.RequestInitChain) abcitypes.ResponseInitChain {

	// Parse the initial validator set from the RequestInitChain message
	for _, v := range req.Validators {
		valAccount := make([]byte, 40)

		pkey := valAccount[8:]

		pk, err := encoding.PubKeyFromProto(v.PubKey)
		if err != nil {
			panic(err)
		}

		copy(pkey, pk.Bytes())

		power := valAccount[:8]
		binary.BigEndian.PutUint64(power, uint64(v.Power))

		addr := app.toAddress(pkey)

		// Initialize the application state with the initial validator set
		err = app.validatorTree.Add(addr, valAccount)
		if err != nil {
			panic(err)
		}
	}

	// Return a response indicating success
	return abcitypes.ResponseInitChain{}
}

func (App) ApplySnapshotChunk(req abcitypes.RequestApplySnapshotChunk) abcitypes.ResponseApplySnapshotChunk {
	return abcitypes.ResponseApplySnapshotChunk{}
}

func (app *App) EndBlock(req abcitypes.RequestEndBlock) abcitypes.ResponseEndBlock {

	binary.BigEndian.PutUint64(app.blockheight[:], uint64(req.Height))
	app.blockHeight = req.Height

	return abcitypes.ResponseEndBlock{ValidatorUpdates: app.valUpdates}
}

func (app *App) CheckTx(req abcitypes.RequestCheckTx) abcitypes.ResponseCheckTx {
	logs.log("Checking tx...")
	//var txr Transaction
	tx := new(Transaction)
	code := tx.fetchTx(req.Tx, app)

	if code == 0 {
		code = tx.isValid(app)
	}
	if code == 0 {
		//app.txCacheDb.Put(tx.hash[:], tx.data, nil)
		app.txMap[tx.hash] = tx
	}

	logs.logTx(tx)

	return abcitypes.ResponseCheckTx{Code: code, GasWanted: 1}
}

func (app *App) BeginBlock(req abcitypes.RequestBeginBlock) abcitypes.ResponseBeginBlock {
	app.valUpdates = make([]abcitypes.ValidatorUpdate, 0)
	app.prevHash = req.Header.GetLastBlockId().Hash

	wVal := app.validatorDb.WriteTx()

	valNum := int64(len(req.LastCommitInfo.Votes) * 256)

	//reward validators
	for _, v := range req.LastCommitInfo.Votes {
		var ValUpdate abcitypes.ValidatorUpdate

		_, val, err := app.validatorTree.Get(v.Validator.Address)
		if err != nil {
			panic(err)
		}

		ValUpdate.PubKey, err = app.toPk(val[8:])
		if err != nil {
			panic(err)
		}

		ValUpdate.Power = v.Validator.Power
		//fmt.Println("prePOWER: ", ValUpdate.Power)
		//increased reward for proposer
		if bytes.Equal(v.Validator.Address, req.Header.ProposerAddress) {
			//fmt.Println("REWARD!!! +", app.blockReward)
			ValUpdate.Power += app.blockReward + int64(app.totalFees)
		} else if v.SignedLastBlock {
			continue
		}
		//inactive validators leak power (and money)
		if ValUpdate.Power > app.emptyVoteLeak {
			ValUpdate.Power -= app.emptyVoteLeak * ValUpdate.Power / valNum
		}
		//fmt.Println("postPOWER: ", ValUpdate.Power)
		app.valUpdates = append(app.valUpdates, ValUpdate)

		binary.BigEndian.PutUint64(val[:8], uint64(ValUpdate.Power))
		err = app.validatorTree.UpdateWithTx(wVal, v.Validator.Address, val)
		if err != nil {
			panic(err)
		}
	}

	//punish byzantines
	for _, b := range req.ByzantineValidators {
		var ValUpdate abcitypes.ValidatorUpdate
		_, val, err := app.validatorTree.Get(b.Validator.Address)
		if err != nil {
			panic(err)
		}
		ValUpdate.PubKey, err = app.toPk(val[8:])
		if err != nil {
			panic(err)
		}
		ValUpdate.Power = 0
		app.valUpdates = append(app.valUpdates, ValUpdate)

		binary.BigEndian.PutUint64(val[:8], uint64(0))
		err = app.validatorTree.UpdateWithTx(wVal, b.Validator.Address, val)
		if err != nil {
			panic(err)
		}
	}

	wVal.Commit()
	wVal.Discard()

	app.totalFees = 0

	return abcitypes.ResponseBeginBlock{}
}

func (app *App) DeliverTx(req abcitypes.RequestDeliverTx) abcitypes.ResponseDeliverTx {
	//
	logs.log("Delivering tx...")
	tx := new(Transaction)
	code := tx.fetchTx(req.Tx, app)
	if code == 0 {
		if !tx.inCache(app) {
			code = tx.isValid(app)
		} else {
			tx = app.txMap[tx.hash]
			code = tx.verifyAccounts(app)
		}
	}

	logs.logTx(tx)

	if code != 0 {
		return abcitypes.ResponseDeliverTx{Code: code}
	}

	if tx.isUpdate {
		logs.log("	update")
		tx.execUpdate(app)
	}

	if tx.isTransfer {
		logs.log("	transfer")
		tx.execTransfer(app)
	}

	if tx.isStake {
		logs.log("	stake")
		tx.execStake(app)
	}

	if tx.isRelease {
		logs.log("	release")
		tx.execRelease(app)
	}

	var dat []byte

	if tx.isContract {
		logs.log("	contract")
		dat = tx.execContract(app)
	}

	if tx.isAccountCreator {
		logs.log("	create")
		dat = tx.execCreateAccount(app)
	}

	if tx.isAccountKeyChanger {
		logs.log("	change")
		tx.execAccountKeyChanger(app)
	}

	if tx.isBatch {
		logs.log("	batch")
		tx.execBatch(app)
	}

	//add tx to the queue to be included in the tx merkle tree
	var key [8]byte
	copy(key[:4], tx.source)
	copy(key[:4], tx.counter)

	app.txDbKeys = append(app.txDbKeys, key[:])
	app.txDbVals = append(app.txDbVals, tx.hash[:])

	//release space on the map by deleting the processed tx
	delete(app.txMap, tx.hash)

	return abcitypes.ResponseDeliverTx{Code: code, Data: dat}
}

func (app *App) Commit() abcitypes.ResponseCommit {

	//permanent storage of account updates
	app.commitAccountsToDb()

	//write deliver txs results on db
	app.txDbMutex.Lock()
	app.txStorageTree.AddBatch(app.txDbKeys, app.txDbVals)
	app.txDbMutex.Unlock()

	//take the number of total processed accounts
	accountNumOnDb, err := app.accountTree.GetNLeafs()
	if err != nil {
		panic(err)
	}
	app.accountNumOnDb = accountNumOnDb

	//permanent storage of contract updates
	app.commitContractsToDb()

	//take the number of total processed contracts
	contractNumOnDb, err := app.contractTree.GetNLeafs()
	if err != nil {
		panic(err)
	}
	app.contractNumOnDb = contractNumOnDb

	//reset batches
	app.txDbKeys = make([][]byte, 0)
	app.txDbVals = make([][]byte, 0)

	//get merkle roots of the trees
	ledgerRoot, err := app.accountTree.Root()
	if err != nil {
		panic(err)
	}
	app.txDbMutex.Lock()
	blockRoot, err := app.txStorageTree.Root()
	app.txDbMutex.Unlock()
	if err != nil {
		panic(err)
	}
	result := app.poseidon(blockRoot)

	validatorRoot, err := app.validatorTree.Root()
	if err != nil {
		panic(err)
	}
	byteSlice := append(ledgerRoot, validatorRoot...)

	//sha256 hash of the roots
	resthash := app.sha2(byteSlice)

	//add it as a block hash to the blockhash tree
	err = app.blockHashTree.Add(app.blockheight[:], result)
	if err != nil {
		//error
		logs.logError("BlockHashTree Error: ", err)
		panic(err)
	}

	//take the root to commit it
	chainroot, err := app.blockHashTree.Root()
	if err != nil {
		panic(err)
	}

	resp := append(resthash, chainroot...)
	//fmt.Println("Commit: ", ledgerRoot, blockRoot, resp)

	// Create a new ResponseCommit message with the data and retainHeight values
	response := abcitypes.ResponseCommit{
		Data: resp,
	}

	//swap and clear old databases
	err = app.swapDb()
	if err != nil {
		panic(err)
	}

	// Return the ResponseCommit message
	return response
}

func (app *App) Query(reqQuery abcitypes.RequestQuery) (resQuery abcitypes.ResponseQuery) {
	resQuery.Key = reqQuery.Data
	key := reqQuery.Data
	logs.log("Query:")
	logs.log(key)
	value := key

	switch len(key) {
	case 1:
		value = app.prevHash
	case 2:
		app.accountWatch = !app.accountWatch
	case 4:
		account, err := app.fetchAccount(key)
		if err == nil {
			value = account.Data
		}
	case 8:
		app.txDbMutex.Lock()
		_, val1, val2, _, err := app.txStorageTree.GenProof(key)
		if err != nil {
			_, val1, val2, _, err = app.txStorageTree2.GenProof(key)
		}
		app.txDbMutex.Unlock()
		key = val1
		value = val2
		//value = append(val1, val2...)
		//value = append(value, val3...)
		if err != nil {
			value = key
		}
	case 32:
		contract, err := app.findContractBypHash(key)
		if err == nil {
			value = contract.Address
		}
	case 48:
		logs.log("By key...")
		account, err := app.findAccountByPubKey(key)
		if err == nil {
			value = account.Address
		}
	default:
		logs.log("DEFAULT")
		value = key
	}

	return abcitypes.ResponseQuery{
		Code:  0,
		Key:   key,
		Value: value,
	}
}
