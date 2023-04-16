package main


import (
	"crypto/sha256"
	"errors"
	blst "github.com/supranational/blst/bindings/go"
	abcitypes "github.com/tendermint/tendermint/abci/types"
	"bytes"
	"fmt"
	"os"
	//"crypto/rand"
	"kvstore/poseidon"
	"github.com/syndtr/goleveldb/leveldb"
	//"strconv"
        "github.com/tendermint/tendermint/crypto/ed25519"
        "github.com/tendermint/tendermint/crypto/encoding"
	//"encoding/binary"
	//"math/big"
	"encoding/binary"
	"github.com/tendermint/tendermint/proto/tendermint/crypto"
	//qt "github.com/frankban/quicktest"
	"go.vocdoni.io/dvote/db"
	badb "go.vocdoni.io/dvote/db/badgerdb"
	//"github.com/dgraph-io/badger"
	"github.com/vocdoni/arbo"
	//"go.vocdoni.io/dvote/censustree"
	//"go.vocdoni.io/dvote/censustree/arbotree"
)

type App struct {
	ndb     		*leveldb.DB
	txCacheDb     		*leveldb.DB
	accountLedgerDb		*leveldb.DB
	contractLedgerDb	*leveldb.DB
	accountDb		*badb.BadgerDB
	blockTxDb		*badb.BadgerDB
	blockHashDb		*badb.BadgerDB
	validatorDb		*badb.BadgerDB
	accountTree		*arbo.Tree
	blockTxTree		*arbo.Tree
	blockHashTree		*arbo.Tree
	validatorTree		*arbo.Tree
	txMap			map[[32]byte]*Transaction
	tempAccountMap		map[[4]byte]*Account


	//wTx	db.WriteTx
	accountWatch	bool
}

type Entry struct {
	isContract bool
	balance []byte
	schnorrPk []byte
	BlsPk []byte
	data []byte
}

type Transaction struct {
	signature []byte
	data []byte
	source []byte
	target []byte
	amount []byte
	state []byte
	publickeys []byte
	blspk []byte
	pop []byte
	multisignature []byte
	batchsize []byte
	batch []byte
	
	hash [32]byte
	pubkey []byte
	sourceAmount []byte
	counter []byte
	
	Amount uint32
	length int
	
	isAccountCreator bool
	isAccountKeyChanger bool
	isContractCreator bool
	isContract bool
	isUpdate bool
	isTransfer bool
	isBatch bool
	isStake bool
	isDelegate bool
	isRelease bool
}

type Account struct {
	Address []byte
	Data []byte
	State []byte
	schnorrPubKey []byte
	blsPubKey []byte
	pop []byte
	counter []byte
	Counter uint32
	Amount  uint32
	Modified bool
}

type PublicKey = blst.P1Affine
type Signature = blst.P2Affine
type AggregateSignature = blst.P2Aggregate
type AggregatePublicKey = blst.P1Aggregate

var ValUpdates []abcitypes.ValidatorUpdate

var gas = uint32(100)

var prevHash []byte
var blockHeight [8]byte
var BlockHeight int64
var votereward = int64(1)
var blockreward = int64(10)

var txDbKeys, txDbVals [][]byte

func NewApp() (*App, error) {
	app := &App{}
	
	//initialize goleveldb databases
	ndb, err := leveldb.OpenFile("db", nil)
	if err != nil {
		return nil, err
	}
	txCacheDb, err := leveldb.OpenFile("txCacheDb", nil)         
	if err != nil {                                  
        	return nil, err                              
	}
	accountLedgerDb, err := leveldb.
	OpenFile("AccountLedgerDb", nil)         
	if err != nil {                                  
        	return nil, err                              
	}
	contractLedgerDb, err := leveldb.OpenFile("contractLedgerDb", nil)
        if err != nil {
                return nil, err
        }
	
	// create badger databases and associated arbo merkle trees
	// create new Tree of accounts with maxLevels=48 and Blake2b hash function
	accountDb, accountTree, err := app.createTreeDb("badg1", 48, false)
	if err != nil {
	    	fmt.Println("accountTree ledger failed!!!", err)
		return nil, err
	}

	// create Tree of transactions and a write Tx buffer 
	blockTxDb, blockTxTree, err := app.createTreeDb("badg2", 64, true)
	//wTx := blockTxDb.WriteTx()

	if err != nil {
	    	// handle error
	    	fmt.Println("Tree blockTxTree failed!!!", err)
		return nil, err
	}
	
	//create a tree of blockhashes
	blockHashDb, blockHashTree, err := app.createTreeDb("badg3", 64, true)
	if err != nil {
	    	fmt.Println("Tree blockHashTree failed!!!", err)
		return nil, err
	}
	
	validatorDb, validatorTree, err := app.createTreeDb("badg4", 256, false)
	if err != nil {
		fmt.Println("Validafor Tree failed", err)
	}
	
	//initialize maps for temporary storage and fast access for transactions and accounts
	txMap := make(map[[32]byte]*Transaction)
	tempAccountMap := make(map[[4]byte]*Account)
	
	//load cached transactions from storage to the maps 
	iter := txCacheDb.NewIterator(nil, nil)
	defer iter.Release()

	for iter.Next() {
    		// Load the entry value into a Transaction instance
    		tx := new(Transaction)
 		tx.data = iter.Value()

    		// Check if the entry already exists in the txMap
    		key := tx.hash
    		if _, ok := txMap[key]; !ok {
        		// If not, add it to the map and perform the checks
        		if app.selectTxType(tx) {
            			if app.hasValidAccounts(tx) == 0 {
        				txMap[key] = tx
				}
			}
   		 }
	}
	if err := iter.Error(); err != nil {
    		// handle error
		panic(err)
	}
	
	// toggle for watching accounts (register a db with bls public keys as db keys
	acW := true
	
	//constructing the app
	app = &App{                                                   
		ndb: ndb,                                               
		txCacheDb: txCacheDb,                                 
		accountLedgerDb: accountLedgerDb,                     
		contractLedgerDb: contractLedgerDb,  
		accountDb: accountDb,
		blockTxDb: blockTxDb,
		blockHashDb: blockHashDb,
		validatorDb: validatorDb,
		accountTree: accountTree,
		blockTxTree: blockTxTree,
		blockHashTree: blockHashTree,
		validatorTree: validatorTree,  
		txMap: txMap,            
		tempAccountMap: tempAccountMap,           
		//wTx: wTx,  
		accountWatch: acW,        
	}			
	
	//example account creation
	
	//the following code is dirty. To be deleted on production
	
	pad := make([]byte, 64)
	
	//first example account creation             

        privkey := ed25519.GenPrivKeyFromSecret([]byte("Iloveyou!"))                    
        pubkey := privkey.PubKey()                                                     
        firstindex := []byte{0, 0, 0, 0}                                               
	amount := []byte{1, 0, 0, 0}                                               
        accountBytes := append(amount, pubkey.Bytes()...)      
        accountBytes = append(accountBytes, pad...)
        //accountBytes = append(accountBytes, pad...)
	account := new(Account)
	
	account.Data = accountBytes     
	account.Address = firstindex
	account.Amount = 500000
	
	app.writeAccount(account)
	
	
        //second example account creation 
	
	privkey = ed25519.GenPrivKeyFromSecret([]byte("Iloveher"))  
        pubkey = privkey.PubKey()                                   
        firstindex = []byte{0, 0, 0, 1}                             
        amount = []byte{1, 0, 0, 0}                                 
        accountBytes = append(amount, pubkey.Bytes()...)  
        accountBytes = append(accountBytes, pad...)
        //accountBytes = append(accountBytes, pubkey.Bytes()...)
	account2 := new(Account)        
	                       
	account2.Data = accountBytes                           
	account2.Address = firstindex
        account2.Amount = 500000              
	                 
	app.writeAccount(account2)
	
	app.commitAccountsToDb()
	
	return app, nil
}

var _ abcitypes.Application = (*App)(nil)

func (app *App) destroyDb(dbpoint *badb.BadgerDB, dbname string) (error) {
    	// Close any existing database and delete the files
    	err := dbpoint.Close()
    	if err != nil {
    		fmt.Println("Failed to close database ", dbname, " !!!", err)
        	return  err
    	}

    	// Remove the directory and its contents
    	err = os.RemoveAll(dbname)
    	if err != nil {
    		fmt.Println("Failed to remove database ", dbname, " files!!!", err)
        	return  err
    	}
	return nil
}

func (app *App) createTreeDb(dbname string, levels int, poseidon bool) (*badb.BadgerDB, *arbo.Tree, error) {
    	// Create a new database
	var opts db.Options
    	opts.Path = dbname
        dbpoint, err := badb.New(opts)         
    	if err != nil {       
    		fmt.Println("Failed to access database ", dbname, " !!!", err)
             	return nil, nil, err
	}

    	// Create a new tree associated with the database
	var config arbo.Config

	if poseidon {
    		config = arbo.Config{
        		Database:     dbpoint,
        		MaxLevels:    levels,
        		HashFunction: arbo.HashFunctionPoseidon}
	} else {
    		config = arbo.Config{
        		Database:     dbpoint,
        		MaxLevels:    levels,
        		HashFunction: arbo.HashFunctionBlake2b}
    	}
	
	Tree, err := arbo.NewTree(config)
	
    	if err != nil {
    		fmt.Println("Failed to create tree ", dbname, " !!!", err)
        	return dbpoint, nil, err
    	}

    	return dbpoint, Tree, nil
}


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
			fmt.Println("hey")
			panic(err)
			fmt.Println("ho")
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
	
func (App) EndBlock(req abcitypes.RequestEndBlock) abcitypes.ResponseEndBlock {

	binary.BigEndian.PutUint64(blockHeight[:], uint64(req.Height))
	BlockHeight = req.Height
	
	return abcitypes.ResponseEndBlock{ValidatorUpdates: ValUpdates}
}

func (app *App) isSigned(tx *Transaction) (code bool) {
	//load public key from account database
	fmt.Println("Enter isSigned...")    
	account, err := app.fetchAccount(tx.source)                        
	
	if err != nil {
		return false
	}
	tx.pubkey = account.schnorrPubKey
	pubkey := ed25519.PubKey(tx.pubkey)
	
	tx.counter = account.counter
	
	fmt.Println("Schnorr pubkey:", pubkey, len(pubkey.Bytes()))
	fmt.Println("Schnorr signature:")                            
        fmt.Println(tx.signature)             
	
	hash := hashData(append(tx.hash[:], account.counter...))
	                               
        fmt.Println("Poseidon Hash:")                                
        fmt.Println(hash, tx.hash)
	fmt.Println("counter: ", account.counter)
	

        if !pubkey.VerifySignature(hash[:], tx.signature) {
                fmt.Println(pubkey, tx.signature, tx.hash)
		return false
        } else {
                //fmt.Println("Valid!")                          
                //fmt.Println(privkey,pubkey,msg)          
		return true      
        }
	
	return true
}

func (app *App) selectTxType(tx *Transaction) (code bool) {
	fmt.Println("Type?")                            
	tx.source = tx.data[:4]	
	
	//transaction format and type defined by the tx blob size
	switch tx.length {
	case 100:
		tx.source = tx.data[:4]
                tx.state = tx.data[4:]
		tx.isUpdate = true
		return true
       
	case 68: //releases funds from staking (or delegation)
		tx.source = tx.data[:4]	
                tx.isRelease = true
		return true
	
	case 72:
		tx.source = tx.data[:4]
		tx.amount = tx.data[4:8]	
                tx.isStake = true
		return true
		
	case 74:
		tx.source = tx.data[:4]	
		tx.amount = tx.data[8:10]
		tx.target = tx.data[4:8]
                tx.isDelegate = true
		return true
        
	case 76:
		tx.source = tx.data[:4]	
		tx.target = tx.data[4:8]
		tx.amount = tx.data[8:12]
                tx.isTransfer = true
		return true
		
	case 108:                   
		tx.source = tx.data[:4]
                tx.target = tx.data[4:8]    
		tx.amount = tx.data[8:12]
		tx.state = tx.data[12:]
                tx.isTransfer = true
		fmt.Println("TRANSFERING...")
		return true

	case 244: // tx change account keys 
		tx.source = tx.data[:4]  
		tx.target = nil
		tx.amount = nil
		tx.publickeys = tx.data[4:]   
		tx.isAccountKeyChanger = true            
		return true

        case 248: // tx create account
		tx.source = tx.data[:4]	
		tx.target = nil
		tx.amount = tx.data[4:8]
                tx.publickeys = tx.data[8:]
                tx.isAccountCreator = true
		return true
		
        default:                                                      
		if tx.length < 13 {
			return false
		}
        }
	/*
	tx.batchsize = tx.data[12:13]
	bs, _ := strconv.Atoi(string(tx.batchsize))
	
	
	if bs == 0 {
		tx.isContract = true
		return true
	}
	
	if tx.length < 157 {
                return false
        }

	tx.batch = tx.data[125:]
	
	if len(tx.batch) == 4*bs {
		tx.isBatch = true
		return true
	}
	*/
	return false	
}

func (app *App) blsCompressedVerify(sig *Signature, sg, blspk, msg, dst []byte) bool {
       	fmt.Println("Verifying Bls...")                            
	if sig.VerifyCompressed(sg, false, blspk, true, msg, dst) {
                fmt.Println("Pop Valid!")
                return true
        } else {                                                             
                fmt.Println("fuck")                                          
                return false                                                 
        }                                                                    
}

func (app *App) sha2(data []byte) []byte {
	//proof of possesion
        h := sha256.New()                           
	h.Write(data)
        sha_2 := h.Sum(nil)
	return sha_2
}

func (app *App) verifyTxPop(tx *Transaction) bool {
	var dst = []byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_")
	dummySig := new(Signature)
	
	//hash mecessary tx data
	h := sha256.New()
        h.Write(tx.blspk)
        h.Write(tx.source)
	h.Write(tx.counter)
        hash := h.Sum(nil)
	
	fmt.Println("POP: ", tx.pop)
	fmt.Println("BLSPK: ", tx.blspk)
	fmt.Println("___", tx.source, tx.counter, hash)
	return app.blsCompressedVerify(dummySig, tx.pop, tx.blspk, hash, dst)
}

func (app *App) verifyBlsTx(tx *Transaction) (bool) {
	if tx.isAccountCreator {
		//fetch proof of posession and the public key
		tx.pop = tx.data[88:]
		tx.blspk = tx.data[40:88]
		
		return app.verifyTxPop(tx)
	}
	
	if tx.isAccountKeyChanger {
		//fetch proof of posession and the public key
		tx.pop = tx.data[84:]
		tx.blspk = tx.data[36:84]
		
		return app.verifyTxPop(tx)
	} 	
		
	return true
}

func (app *App) inCache(tx *Transaction) (bool) {
    	// Check the map first
    	value, found := app.txMap[tx.hash]
    	if found == false  {
		/*
    		// Fall back to the cache database
    		value, err = app.txCacheDb.Get(tx.hash, nil)
    		if err != nil {
        		// handle error, for example:
        		// return error or log it
        		return false
    		}
		*/
		return false
	}
	
    	if bytes.Equal(tx.data, value.data) {
        	// Update the map with the transaction if it's found in the cache database
        	//app.txMap[... 
        	return true
    	}

    	return false
}


func (app *App) hasValidAccounts(tx *Transaction) (code uint32) {
	fmt.Println("Has valid accounts?")                            
        //value, err := app.accountLedgerDb.Get(tx.source, nil)
	
	account, err := app.fetchAccount(tx.source)
	if err != nil {
		return 17
	}
	//tx.sourceAmount = value[:4]
	
	if tx.target != nil {
	        _, err = app.fetchAccount(tx.target)  
	}
	
	if err != nil {
		return 18
	}
	
	if tx.amount != nil {
        	tx.Amount = binary.BigEndian.Uint32(tx.amount)    
	}
	   
	if tx.Amount + (gas * uint32(tx.length)) > account.Amount {
		return 23
	}
	
	return 0
}

func (app *App) fetchTx(rawtx []byte, tx *Transaction) (code uint32) {
	// check format
	tx.length = len(rawtx)
	
	if tx.length > 1401 { //tx max size check
                return 1
        }
	
	if tx.length < 68 { // tx min size check
                return 2
        }

	tx.signature = rawtx[:64]
	tx.data = rawtx[64:]
	hash := hashData(tx.data)
	copy(tx.hash[:], hash)
		fmt.Println(tx.data, tx.signature, tx.length)
	return 0
}
	
func (app *App) isValid(tx *Transaction) (code uint32) {
	fmt.Println("Is valid?")                            
	if app.inCache(tx) {
		return 33
	}
	
	if !app.selectTxType(tx) {
		return 44
	}
	
	if !app.isSigned(tx) {
		return 39
	}
	
	if !app.verifyBlsTx(tx) {
		return 89
	}
	
	return app.hasValidAccounts(tx)	

	return 11
}

func (app *App) CheckTx(req abcitypes.RequestCheckTx) abcitypes.ResponseCheckTx {
	fmt.Println("Checking tx...")                            
	//var txr Transaction
	tx := new(Transaction)
	code := app.fetchTx(req.Tx, tx)
	fmt.Println(tx.data, tx.signature, tx.length)
	if code == 0 {
		code = app.isValid(tx)
	}
	if code == 0 {
		app.txCacheDb.Put(tx.hash[:], tx.data, nil)
		app.txMap[tx.hash] = tx
	}
	return abcitypes.ResponseCheckTx{Code: code, GasWanted: 1}
}



func (app *App) BeginBlock(req abcitypes.RequestBeginBlock) abcitypes.ResponseBeginBlock {
	ValUpdates = make([]abcitypes.ValidatorUpdate, 0)
	prevHash=req.Header.GetLastBlockId().Hash

	
	//reward validators
    	for _, v := range req.LastCommitInfo.Votes {
		var ValUpdate abcitypes.ValidatorUpdate
		
		if !v.SignedLastBlock {
			continue
		}
		
		_, val, err := app.validatorTree.Get(v.Validator.Address)
		if err != nil {
			panic(err)
		}
		
		ValUpdate.PubKey, err = app.toPk(val[8:])
		if err != nil {
			panic(err)
		}
		
		ValUpdate.Power = v.Validator.Power+votereward
		
		//increased reward for proposer
		if bytes.Equal(v.Validator.Address, req.Header.ProposerAddress) {
			ValUpdate.Power += blockreward
		}
		
		ValUpdates = append(ValUpdates, ValUpdate)
		
		binary.BigEndian.PutUint64(val[:8], uint64(ValUpdate.Power))
		err = app.validatorTree.Update(v.Validator.Address, val)
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
		ValUpdates = append(ValUpdates, ValUpdate)
		
		binary.BigEndian.PutUint64(val[:8], uint64(0))
		err = app.validatorTree.Update(b.Validator.Address, val)
		if err != nil {
			panic(err)
		}
	}
	
	return abcitypes.ResponseBeginBlock{}
}

func (app *App) fetchAccount(address []byte) (*Account, error) {
	account := &Account{}
	fmt.Println("fetching...")
	
	//maybe a previous tx has been delivered but not yet written to db 
        //check the map of temp accounts for a changed amount
        var key [4]byte
        copy(key[:], account.Address)
                
        v, ok := app.tempAccountMap[key]
        if ok {
                account = v
		return account, nil
        }
	
	_, value, err := app.accountTree.Get(address)
	if err != nil {
		return nil, errors.New("Miss accountTree entry")
	}
	if len(value) < 36 {
		return nil, errors.New("Not enough data, some error occurred")
	}
	account.Data = value[:]
	account.Address = address
	account.schnorrPubKey = value[4:36]
	account.Amount = binary.BigEndian.Uint32(account.Data[:4])
	
	fmt.Println("fetched data: ", account.Data)
	
	if len(value) < 88 {
		account.Counter = 0
	} else {
		account.counter = account.Data[84:88]
		account.Counter = binary.BigEndian.Uint32(account.counter)
	}
	
	if len(value) == 120 {
		account.State = value[88:]
	}
	
	return account, nil
}

func (app *App) writeAccount(account *Account) {
	fmt.Println("Writting...")
	newData := make([]byte, 88)
	
	binary.BigEndian.PutUint32(newData[:4], account.Amount)
	
	binary.BigEndian.PutUint32(newData[84:88], account.Counter)
	
	copy(newData[4:84], account.Data[4:84])
	
	account.Data = append(newData[:88], account.State...)
	
	
	fmt.Println("write account address: ", account.Address)  
	fmt.Println("write account data: ", account.Data)  

	
	//update temp account cache
        var key [4]byte
	copy(key[:], account.Address)
	account.Modified = true
	app.tempAccountMap[key] = account	
}

func (app *App) commitAccountsToDb() {

        wAc := app.accountDb.WriteTx()
	
    	for _, account := range app.tempAccountMap {
        	if account.Modified {
			app.commitAccountToDb(account, wAc)
		}
	}
	//write deliver txs results on db                     
	wAc.Commit()                                      
	wAc.Discard()
}

func (app *App) commitAccountToDb(account *Account, wAc db.WriteTx) {

	//add to the stream to be commited to db
	err1 := app.accountTree.AddWithTx(wAc, account.Address, account.Data)
	if err1 != nil {
		err2 := app.accountTree.UpdateWithTx(wAc, account.Address, account.Data)
		if err2 != nil {
			fmt.Println("Acctree update FATAL ERROR!!!", err1, err2)
		}
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
	err := app.accountLedgerDb.Put(account.Data[36:84], account.Address, nil) 
        if err != nil {     
                //handle err      
                fmt.Println("ACCOUNT DB WRITE ERROR!!!")              
        }	
}



func (app *App) DeliverTx(req abcitypes.RequestDeliverTx) abcitypes.ResponseDeliverTx {
	//
	fmt.Println("Delivering tx...")                            
	tx := new(Transaction)
	code := app.fetchTx(req.Tx, tx)
	if code == 0 {
		if !app.inCache(tx) {
			code = app.isValid(tx)
		} else {
			tx = app.txMap[tx.hash]
			code = app.hasValidAccounts(tx)
		}
	}
	if code != 0 {
		return abcitypes.ResponseDeliverTx{Code: code}
	}
	
	if tx.isUpdate {
		app.execUpdate(tx)
	}
	
	if tx.isTransfer {
		app.execTransfer(tx)
	}
	
	if tx.isStake {
		app.execStake(tx)
	}
	
	if tx.isRelease {
		app.execRelease(tx)
	}
	
	var dat []byte
	
	if tx.isAccountCreator {
		dat = app.execCreateAccount(tx)
	}
	
	if tx.isAccountKeyChanger {
		app.execAccountKeyChanger(tx)
	}
	
	//add tx to the queue to be included in the tx merkle tree
	var key [8]byte
	copy(key[:4], tx.source)
	copy(key[:4], tx.counter)

	txDbKeys = append(txDbKeys, key[:])
	txDbVals = append(txDbVals, tx.hash[:])

	return abcitypes.ResponseDeliverTx{Code: code, Data: dat}
}

func (app *App) execAccountKeyChanger(tx *Transaction) {
	account, err := app.fetchAccount(tx.source)
	if err != nil {
		fmt.Println("Fail")
	}
	account.Data = tx.data
	account.Address = tx.source
	
	//fees
	account.Amount -= (gas * uint32(tx.length))
	
	//Update counter on every tx 
	account.Counter++
	
	app.writeAccount(account)
}

func (app *App) toPk(key []byte) (crypto.PublicKey, error) {
	pke := ed25519.PubKey(key)

	pkp, err := encoding.PubKeyToProto(pke)
	if err != nil {
		panic(err)
		return pkp, err
	}
	return pkp, nil
}

func (app *App) toAddress(key []byte) ([]byte) {
	keyhash := app.sha2(key)
	return keyhash[:20]
}

func (app *App) execRelease(tx *Transaction) {
	var val abcitypes.ValidatorUpdate
	val.Power = 0
	
	account := app.execUpdate(tx)
	
	val.PubKey,_ = app.toPk(account.schnorrPubKey)
	
	addr := app.toAddress(account.schnorrPubKey)
	
	_, valAccount, err := app.validatorTree.Get(addr)
	if err != nil {
		return
	}
	
	//power := make([]byte, 8)
	binary.BigEndian.PutUint64(valAccount[:8], uint64(0))
	app.validatorTree.Update(addr, valAccount)
	
	ValUpdates = append(ValUpdates, val)
	Amount := binary.BigEndian.Uint32(valAccount[:8])
	account.Amount = Amount 
	app.writeAccount(account)
}

func (app *App) execStake(tx *Transaction) {
	account := app.execUpdate(tx)
	
	var val abcitypes.ValidatorUpdate
	val.Power = int64(tx.Amount)
	
	val.PubKey,_ = app.toPk(account.schnorrPubKey)
	addr := app.toAddress(account.schnorrPubKey)
	
	_, valAccount, err := app.validatorTree.Get(addr)
	if err == nil {
		val.Power += int64(binary.BigEndian.Uint64(valAccount[:8]))
	} else {
		valAccount = make([]byte, 40)
	}
	binary.BigEndian.PutUint64(valAccount[:8], uint64(val.Power))
	
	if err == nil {
		app.validatorTree.Update(addr, valAccount)
	} else {
		copy(valAccount[8:], account.schnorrPubKey)
		app.validatorTree.Add(addr, valAccount)
	}
	
	ValUpdates = append(ValUpdates, val)
}

func (app *App) execCreateAccount(tx *Transaction) []byte {
	fmt.Println("Creating new account...")                            
	account := new(Account)
	account.Data = tx.data[4:88] 
	
	//find next account address
	nextaddr, err := app.accountTree.GetNLeafs() 
	if err != nil {
		panic(err)
	}
	account.Address = make([]byte, 4)
	binary.BigEndian.PutUint32(account.Address, uint32(nextaddr))
	
	//change account entry
	app.writeAccount(account)
	
	tx.target = account.Address
	app.execTransfer(tx)
	
	return account.Address
}

func (app *App) execUpdate(tx *Transaction) *Account {    
	/// update source account
	// Fetch account
	account, err := app.fetchAccount(tx.source)
	if err != nil {
		//this should not happen
		return nil
	}
	
	// Subtract amount from account
	account.Amount -= (tx.Amount + gas * uint32(tx.length))
		
	//Update counter on every tx
	account.Counter++
		
	//update state
	account.State = tx.state

	// Write updated account to database
	app.writeAccount(account)
	
	return account
}	
	
func (app *App) execTransfer(tx *Transaction) {
	// update target account
	// Fetch account                        
	app.execUpdate(tx)
	                     
        account, err := app.fetchAccount(tx.target)                  
        if err != nil {                                              
                //this should not happen                             
        } else {                                                     
                // Subtract amount from account                      
                account.Amount += tx.Amount                           
		            
                // Write updated account to database                 
                app.writeAccount(account)                      
        }
}

func hashData(data []byte) ([]byte) {
	h2, err := poseidon.HashBytes(data)
	if err != nil {
		fmt.Println("POSEIDON ERROR________________________ _ _ _")
		panic(err)
	}
	
	//bLen := 32 //app.blockHashTree.HashFunction().Len()
	
	result := arbo.BigIntToBytes(32, h2)
	return result  //h2.Bytes()
}

func (app *App) Commit() abcitypes.ResponseCommit {
	 
	byteSlice := make([]byte, 96)
	var resp [96]byte
	
	//permanent storage of account updates
	app.commitAccountsToDb()
	
	//write deliver txs results on db
	app.blockTxTree.AddBatch(txDbKeys, txDbVals)
	
	//reset account map	
	app.tempAccountMap = make(map[[4]byte]*Account)	
	
	//get merkle roots of the trees
	ledgerRoot,_ := app.accountTree.Root()
	copy(byteSlice[:32], ledgerRoot)
	
	blockRoot,_ := app.blockTxTree.Root()
	copy(byteSlice[32:64], blockRoot)
	
	validatorRoot,_ := app.validatorTree.Root()
	copy(byteSlice[64:], validatorRoot)
	
	//poseidon hash of the roots
	result := hashData(byteSlice)
	
	//add it as a block hash to the blockhash tree
	err := app.blockHashTree.Add(blockHeight[:], result)
	if err != nil {
		//error
		fmt.Println("BlockHashTree Error: ", err)
	}

	//take the root to commit it
	root,_ := app.blockHashTree.Root()
	copy(resp[:], root)
	fmt.Println("Commit: ", ledgerRoot, blockRoot, resp)
	
	// Create a new ResponseCommit message with the data and retainHeight values
    	response := abcitypes.ResponseCommit{
     		Data: resp[:],
	}
	
	//reset tx database periodically
	if BlockHeight % 1024 == 0 {
		_ = app.destroyDb(app.blockTxDb, "badg2")
		app.blockTxDb, app.blockTxTree, _ = app.createTreeDb("badg2", 64, true)
	}
	
	// Return the ResponseCommit message
	return response
}

func (app *App) Query(reqQuery abcitypes.RequestQuery) (resQuery abcitypes.ResponseQuery) {
	resQuery.Key = reqQuery.Data
	key := reqQuery.Data
	fmt.Println(key)
	value := key
	
	switch len(key) {
	case 4:
		account, err := app.fetchAccount(key)
		if err == nil {
			value = account.Data
		}
	case 8:
		_, val1, val2, _, err := app.blockTxTree.GenProof(key)
		value = append(val1, val2...)
		//value = append(value, val3...)
		if err != nil {
			value = key
		}
	case 48:
		fmt.Println("By key...")
		account, err := app.findAccountByPubKey(key)
		if err == nil {
			value = account.Address
		}
	default:
		fmt.Println("DEFAULT")
		value = key
	}
	
	
	
	return abcitypes.ResponseQuery{
		Code: 0,
		Key: key,
		Value: value,
	}
}


func (app *App) findAccountByPubKey(blskey []byte) (*Account, error) {
	
	address, err := app.accountLedgerDb.Get(blskey, nil)	
	if err != nil {
		return nil, err
	}
	
	account, err := app.fetchAccount(address)
	
	fmt.Printf("Found account: ", account.Data, "err: ", err)
	
	if err != nil {
		return nil, err
	}	
	
	return account, nil
	
}



