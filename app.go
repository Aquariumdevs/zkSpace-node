package main


import (
	"crypto/sha256"
	"errors"
	blst "github.com/supranational/blst/bindings/go"
	abcitypes "github.com/tendermint/tendermint/abci/types"
	"bytes"
	"fmt"
	"os"
	"sync"
	//"crypto/rand"
	"kvstore/poseidon"
	//"github.com/syndtr/goleveldb/leveldb"
	//"strconv"
        "github.com/tendermint/tendermint/crypto/ed25519"
        "github.com/tendermint/tendermint/crypto/encoding"
	//"encoding/binary"
	//"math/big"
	"encoding/binary"
	"github.com/tendermint/tendermint/proto/tendermint/crypto"
	"go.vocdoni.io/dvote/db"
	badb "go.vocdoni.io/dvote/db/badgerdb"
	"github.com/vocdoni/arbo"
)

type App struct {
	
	//txCacheDb     	*leveldb.DB
	accountLedgerDb		*badb.BadgerDB 
	contractDb		*badb.BadgerDB
	contractStorageDb	*badb.BadgerDB
	contractStorageDb2	*badb.BadgerDB
	accountDb		*badb.BadgerDB
	txStorageDb		*badb.BadgerDB
	txStorageDb2		*badb.BadgerDB
	blockHashDb		*badb.BadgerDB
	validatorDb		*badb.BadgerDB
	accountTree		*arbo.Tree
	contractTree		*arbo.Tree
	txStorageTree		*arbo.Tree
	txStorageTree2		*arbo.Tree
	blockHashTree		*arbo.Tree
	validatorTree		*arbo.Tree
	txMap			map[[32]byte]*Transaction
	tempAccountMap		map[[4]byte]*Account
	tempContractMap		map[[4]byte]*Contract
	tempNewContractMap	map[[4]byte]*Contract
	accountWatch		bool
	
	blockheight 		[8]byte
	blockHeight 		int64
		
	prevHash 		[]byte
	txDbMutex 		sync.Mutex
	ctxDbMutex		sync.Mutex
	valUpdates 		[]abcitypes.ValidatorUpdate
	
	txDbKeys, txDbVals 	[][]byte
	
	accountNumOnDb		int
	contractNumOnDb		int
	
	dummySig		*Signature
	dummyPk			*PublicKey
	
	emptyVoteLeak		int64
	gas 			uint32 
	blockReward		int64
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
	payload []byte
	
	hash [32]byte
	pubkey []byte
	sourceAmount []byte
	counter []byte
	pad byte
	
	//boolArray []bool
	addresses []byte
	batchedTxNum int
	
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
	isEvidence bool //this includes 2 distinct signatures on the same blockheight
			//or on the same tx counter, which indicates that the user tried 
			//to confuse the system. It will impose punishment. this is 
			//absolutely necessary for securing batchers of transactions 
			//against spamming from malicious users
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

type Contract struct {
	Address []byte
	counter []byte
	Counter uint64
	Payload []byte
}

type PublicKey = blst.P1Affine
type Signature = blst.P2Affine
type AggregateSignature = blst.P2Aggregate
type AggregatePublicKey = blst.P1Aggregate


var badg0 string = "badg0"
var badg1 string = "badg1"
var badg2 string = "badg2"
var badg3 string = "badg3"

var contractdb string = "contractdb"
var contractdb2 string = "contractdb2"

func NewApp() (*App, error) {
	app := &App{}
	
	//initialize goleveldb databases
	//txCacheDb, err := leveldb.OpenFile("txCacheDb", nil)         
	//if err != nil {                                  
        //	return nil, err                              
	//}
	
	// create badger databases and associated arbo merkle trees
	accountLedgerDb, err := badb.New(db.Options{Path: "accdb"})          
	if err != nil {                                  
        	return nil, err                              
	}

	// create 2 temporary swaping databases of contract entries	
	contractStorageDb, err := badb.New(db.Options{Path: contractdb})
        if err != nil {
                return nil, err
        }
	
	contractStorageDb2, err := badb.New(db.Options{Path: contractdb2})
        if err != nil {
                return nil, err
	}

	// create new Tree of accounts with maxLevels=48 and Blake2b hash function
	accountDb, accountTree, err := app.createTreeDb("badg", 48, false)
	if err != nil {
	    	fmt.Println("accountTree ledger failed!!!", err)
		return nil, err
	}
	
	// create Tree of contracts
	contractDb, contractTree, err := app.createTreeDb(badg0, 48, false)
	if err != nil {
	    	fmt.Println("contractTree ledger failed!!!", err)
	}
	
	// create 2 temporary swaping Trees of transactions
	
	txStorageDb, txStorageTree, err := app.createTreeDb(badg2, 64, true)
	if err != nil {
	    	// handle error
	    	fmt.Println("Tree txStorageTree failed!!!", err)
		return nil, err
	}
	txStorageDb2, txStorageTree2, err := app.createTreeDb(badg3, 64, true)
	if err != nil {
	    	fmt.Println("Tree txStorageTree failed!!!", err)
		return nil, err
	}
	
	//create a tree of blockhashes
	blockHashDb, blockHashTree, err := app.createTreeDb("badg4", 64, true)
	if err != nil {
	    	fmt.Println("Tree blockHashTree failed!!!", err)
		return nil, err
	}
	
	validatorDb, validatorTree, err := app.createTreeDb("badg5", 256, false)
	if err != nil {
		fmt.Println("Validafor Tree failed", err)
	}
	
	//initialize maps for temporary storage and fast access for transactions and accounts
	txMap := make(map[[32]byte]*Transaction)
	tempAccountMap := make(map[[4]byte]*Account)
	//app.tempContractMap = make(map[[4]byte]*Contract)	
	//app.tempNewContractMap = make(map[[4]byte]*Contract)	
	
	//initialize batches
	//app.txDbKeys = make([][]byte, 0)
	//app.txDbVals = make([][]byte, 0)
	
	/*
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
	*/
	// toggle for watching accounts (register a db with bls public keys as db keys
	acW := true
	
	//constructing the app
	app = &App{
		//initialize parameters          
		gas: uint32(100),
        	emptyVoteLeak: int64(1),   
		blockReward: int64(100000),                                                       
		//txCacheDb: txCacheDb,                                 
		accountLedgerDb: accountLedgerDb,                     
		contractStorageDb: contractStorageDb,  
		contractStorageDb2: contractStorageDb2,
		contractDb: contractDb,  
		//contractDb2: contractDb2,  
		accountDb: accountDb,
		txStorageDb: txStorageDb,
		txStorageDb2: txStorageDb2,
		blockHashDb: blockHashDb,
		validatorDb: validatorDb,
		accountTree: accountTree,
		contractTree: contractTree,
		//contractTree2: contractTree2,
		txStorageTree: txStorageTree,
		txStorageTree2: txStorageTree2,
		blockHashTree: blockHashTree,
		validatorTree: validatorTree,  
		txMap: txMap,            
		tempAccountMap: tempAccountMap,           
		//wTx: wTx,  
		accountWatch: acW,        
	}			
	
	app.dummySig = new(Signature)
	app.dummyPk = new(PublicKey)
	
	//example account creation
	
	//the following code is dirty. To be deleted on production
	
	pad := make([]byte, 64)
	
	
	//first example account creation             

        privkey := ed25519.GenPrivKeyFromSecret([]byte("Iloveyou!"))                    
        pubkey := privkey.PubKey()                                                     
        firstindex := []byte{0, 0, 0, 0}                                               
	amount := []byte{250, 0, 0, 0}                                               
        accountBytes := append(amount, pubkey.Bytes()...)      
        accountBytes = append(accountBytes, pad...)
        
	account := new(Account)
	account.Data = accountBytes     
	account.Address = firstindex
	account.Amount = 50000000
	
	app.writeAccount(account)
	
	
        //second example account creation 
	
	privkey = ed25519.GenPrivKeyFromSecret([]byte("Iloveher"))  
        pubkey = privkey.PubKey()                                   
        firstindex = []byte{0, 0, 0, 1}                             
        amount = []byte{1, 0, 0, 0}                                 
        accountBytes = append(amount, pubkey.Bytes()...)  
        accountBytes = append(accountBytes, pad...)
        
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
	return app.createTree(dbpoint, levels, poseidon)
}

func (app *App) createTree(dbpoint *badb.BadgerDB, levels int, poseidon bool) (*badb.BadgerDB, *arbo.Tree, error) {
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
    		fmt.Println("Failed to create tree !!!", err)
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
	
	//fmt.Println("Schnorr pubkey:", pubkey, len(pubkey.Bytes()))
	//fmt.Println("Schnorr signature:")                            
        //fmt.Println(tx.signature)             
	
	hash := hashData(append(tx.hash[:], account.counter...))
	                               
        //fmt.Println("Poseidon Hash:")                                
        //fmt.Println(hash, tx.hash)
	//fmt.Println("counter: ", account.counter)
	

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
	//tx.source = tx.data[:4]	//the same on all occasions
	
	//transaction format and type defined by the tx blob size
	switch tx.length {
	case 100:
		//tx.source = tx.data[:4]
                tx.state = tx.data[4:]
		tx.isUpdate = true
		return true
       
	case 68: //releases funds from staking (or delegation)
		//tx.source = tx.data[:4]	
                tx.isRelease = true
		return true
	
	case 72:
		//tx.source = tx.data[:4]
		tx.amount = tx.data[4:8]	
                tx.isStake = true
		return true
		
	case 74:
		//tx.source = tx.data[:4]	
		tx.amount = tx.data[8:10]
		tx.target = tx.data[4:8]
                tx.isDelegate = true
		return true
        
	case 76:
		//tx.source = tx.data[:4]	
		tx.target = tx.data[4:8]
		tx.amount = tx.data[8:12]
                tx.isTransfer = true
		return true
		
	case 108:                   
		//tx.source = tx.data[:4]
                tx.target = tx.data[4:8]    
		tx.amount = tx.data[8:12]
		tx.state = tx.data[12:]
                tx.isTransfer = true
		fmt.Println("TRANSFERING...")
		return true

	case 244: // tx change account keys 
		//tx.source = tx.data[:4]  
		tx.target = nil
		tx.amount = nil
		tx.publickeys = tx.data[4:]   
		tx.isAccountKeyChanger = true            
		return true

        case 248: // tx create account
		//tx.source = tx.data[:4]	
		tx.target = nil
		tx.amount = tx.data[4:8]
                tx.publickeys = tx.data[8:]
                tx.isAccountCreator = true
		return true
	
        default:                                                   
		if tx.length < 73 {
			return false
		}
		//tx.source = tx.data[:4]
		tx.amount = tx.data[4:8]
		tx.pad = tx.data[8]
		if uint8(tx.pad) > 1 {
			tx.isBatch = true
		} else {
			tx.target = tx.data[9:13]
			tx.payload = tx.data[13:]
			tx.isContract = true
		}
		return true
        }
	return false	
}

func (app *App) blsCompressedVerify(sig *Signature, sg, blspk, msg, dst []byte) bool {
       	fmt.Println("Verifying Bls...")                            
	if sig.VerifyCompressed(sg, true, blspk, true, msg, dst) {
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

func (app *App) verify(tx *Transaction) bool {
	return true
}

func (app *App) verifyTxPop(tx *Transaction) bool {
	var dst = []byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_")
	
	//hash mecessary tx data
	h := sha256.New()
        h.Write(tx.blspk)
        h.Write(tx.source)
	h.Write(tx.counter)
        hash := h.Sum(nil)
	
	//fmt.Println("POP: ", tx.pop)
	//fmt.Println("BLSPK: ", tx.blspk)
	//fmt.Println("___", tx.source, tx.counter, hash)
	return app.blsCompressedVerify(app.dummySig, tx.pop, tx.blspk, hash, dst)
}

func (app *App) countBitmap(byteSlice []byte) int {
	//counts the number of true bits in the bitmap
	var counter int
	for i := range byteSlice {
	    	for j := 0; j < 8; j++ {
	        	if byteSlice[i]&(1<<uint(j)) != 0 {
				counter ++
			}
		}
	}
	return counter
}

func (app *App) parseBitmap(byteSlice []byte) ([]bool, int) {
	// convert the byte slice to a bool array
	boolArray := make([]bool, len(byteSlice)*8)
	var counter int
	
	for i := range byteSlice {
	    	for j := 0; j < 8; j++ {
	        	boolArray[i*8+j] = byteSlice[i]&(1<<uint(j)) != 0
			if boolArray[i*8+j] {
				counter ++
			}
	    	}
	}
	return boolArray, counter
}

func (app *App) verifyBatch(tx *Transaction) bool {	
	//check that height recordes into the state is larger than the current height
	maxheight := binary.BigEndian.Uint64(tx.state[32:40])
	if app.blockHeight > int64(maxheight) {
		return false
	}
	
	//parse bitmap
	bMapEnd := int(112 + tx.pad)
	if bMapEnd > len(tx.data) {
		return false
	}
	bitmap := tx.data[113 : bMapEnd]
	tx.batchedTxNum = app.countBitmap(bitmap)
	
	//parse participating  account addresses
	tx.addresses = tx.data[bMapEnd:]
	
	if tx.batchedTxNum * 4  >= len(tx.addresses) {
		return false
	}
	
	//collect public keys
	var cpKeys [][]byte
	
	for i := 0; i < tx.batchedTxNum ; i += 4 {
		address := tx.addresses[i : i  + 4]
		account, err := app.fetchAccount(address)
		if err != nil {
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
	//msg := blst.HashToG2(tx.state, dst)
	return sig.FastAggregateVerify(false, PKeys, tx.state, dst)
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
	
	if tx.isBatch {
		tx.multisignature = tx.data[9:73]
		tx.state = tx.data[73:113]
		return app.verifyBatch(tx)
	}	
		
	return true
}

func (app *App) inCache(tx *Transaction) (bool) {
    	// Check the map first
    	value, found := app.txMap[tx.hash]
    	if found == false  {
		return false
	}
	
    	if bytes.Equal(tx.data, value.data) {
        	return true
    	}

    	return false
}


func (app *App) hasValidAccounts(tx *Transaction) (code uint32) {
	fmt.Println("Has valid accounts?")                            
	
	account, err := app.fetchAccount(tx.source)
	if err != nil {
		return 17
	}
		
	if tx.target != nil {
	        _, err = app.fetchAccount(tx.target)  
	}
	
	if err != nil {
		return 18
	}
	
	if tx.amount != nil {
        	tx.Amount = binary.BigEndian.Uint32(tx.amount)    
	}

	fmt.Println("AMOUNTS: ", tx.Amount, account.Amount)

	if tx.Amount + (app.gas * uint32(tx.length)) > account.Amount {

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
	tx.source = tx.data[:4]
	hash := hashData(tx.data)
	copy(tx.hash[:], hash)
	//	fmt.Println(tx.data, tx.signature, tx.length)
	return 0
}
	
func (app *App) isValid(tx *Transaction) (code uint32) {
	fmt.Println("Is valid?")                            
	if app.inCache(tx) {
		return 33
	}
	
	if !app.isSigned(tx) {
		return 39
	}
	
	if !app.selectTxType(tx) {
		return 44
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
	//fmt.Println(tx.data, tx.signature, tx.length)
	if code == 0 {
		code = app.isValid(tx)
	}
	if code == 0 {
		//app.txCacheDb.Put(tx.hash[:], tx.data, nil)
		app.txMap[tx.hash] = tx
	}
	return abcitypes.ResponseCheckTx{Code: code, GasWanted: 1}
}



func (app *App) BeginBlock(req abcitypes.RequestBeginBlock) abcitypes.ResponseBeginBlock {
	app.valUpdates = make([]abcitypes.ValidatorUpdate, 0)
	app.prevHash = req.Header.GetLastBlockId().Hash

	wVal := app.validatorDb.WriteTx()
	
	valNum := int64(len(req.LastCommitInfo.Votes)*256)
	
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
			ValUpdate.Power += app.blockReward
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
	
	return abcitypes.ResponseBeginBlock{}
}

func (app *App) fetchAccount(address []byte) (*Account, error) {
	account := &Account{}
	fmt.Println("fetching...")
	
	//maybe a previous tx has been delivered but not yet written to db 
        //check the map of temp accounts for a changed amount
        var key [4]byte
        copy(key[:], address)
                
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
	fmt.Println("Writting... new amount: ", account.Amount)
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

func (app *App) commitContractsToDb() {
	app.ctxDbMutex.Lock()
	conBatch := db.NewBatch(app.contractStorageDb)
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
	var newContractKeys, newContractValues	[][]byte
	
    	for _, contract := range app.tempNewContractMap {
		newContractKeys = append(newContractKeys, contract.Address[:])
		newContractValues = append(newContractValues, zerocounter)
		contract.counter = zerocounter[:]
		app.commitContractToDb(contract, conBatch)
	}
	
	//add new contracts to contract tree
	app.contractTree.AddBatch(newContractKeys, newContractValues)
	
	//now write contract payloads to db for data availability
	conBatch.Commit()
	conBatch.Discard()
	
	app.ctxDbMutex.Unlock()
	
	//reset contract maps
	app.tempContractMap = make(map[[4]byte]*Contract)
	app.tempNewContractMap = make(map[[4]byte]*Contract)
}

func (app *App) commitContractToDb(contract *Contract, conBatch *db.Batch) {
	address := append(contract.Address, contract.counter...)
	err := conBatch.Set(address, contract.Payload) 
        if err != nil {
		panic(err)
	}
}

func (app *App) commitAccountsToDb() {
	accBatch := db.NewBatch(app.accountLedgerDb)
        wAc := app.accountDb.WriteTx()
	
    	for _, account := range app.tempAccountMap {
        	if account.Modified {
			app.commitAccountToDb(account, accBatch, wAc)
		}
	}
	//write deliver txs results on db                     
	wAc.Commit()                                      
	wAc.Discard()
	accBatch.Commit()
	accBatch.Discard()
}

func (app *App) commitAccountToDb(account *Account, accBatch *db.Batch, wAc db.WriteTx) {

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
	err := accBatch.Set(account.Data[36:84], account.Address) 
        if err != nil {     
                //handle err      
                fmt.Println("ACCOUNT DB WRITE ERROR!!!")              
        }	
}

func (app *App) execBatch(tx *Transaction) {
	//truncate maxheight from state
	tx.state = tx.state[:32]
	tx.target = tx.source

	for i := 0; i < len(tx.addresses) ; i += 4 {
		tx.source = tx.addresses[i : i  + 4]
		tx.length = 0 //this leads to zero fees fees are paid from the batcer
		app.execUpdate(tx) //the amount will be subtracted from every participant 
	}

        account, err := app.fetchAccount(tx.target)
        if err != nil {
		panic(err)
	}
	
	account.Amount += tx.Amount * uint32(tx.batchedTxNum) - (app.gas * uint32(tx.length))        
	
        app.writeAccount(account)                                                   
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
		fmt.Println("update")
		app.execUpdate(tx)
	}
	
	if tx.isTransfer {
		fmt.Println("transfer")
		app.execTransfer(tx)
	}
	
	if tx.isStake {
		fmt.Println("stake")
		app.execStake(tx)
	}
	
	if tx.isRelease {
		fmt.Println("release")
		app.execRelease(tx)
	}

	var dat []byte
	
	if tx.isContract {
		fmt.Println("contract")
		dat = app.execContract(tx)
	}
	
	if tx.isAccountCreator {
		fmt.Println("create")
		dat = app.execCreateAccount(tx)
	}
	
	if tx.isAccountKeyChanger {
		fmt.Println("change")
		app.execAccountKeyChanger(tx)
	}
	
	if tx.isBatch {
		fmt.Println("batch")
		app.execBatch(tx)
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

func (app *App) execAccountKeyChanger(tx *Transaction) {
	account, err := app.fetchAccount(tx.source)
	if err != nil {
		fmt.Println("Fail")
	}
	account.Data = tx.data
	account.Address = tx.source
	
	//fees
	account.Amount -= (app.gas * uint32(tx.length))
	
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
	var err error
	
	account := app.execUpdate(tx)
	
	val.PubKey, err = app.toPk(account.schnorrPubKey)
	if err != nil {
		panic(err)
	}
	
	addr := app.toAddress(account.schnorrPubKey)
	
	_, valAccount, err := app.validatorTree.Get(addr)
	if err != nil {
		return
	}
	binary.BigEndian.PutUint64(valAccount[:8], uint64(0))
	
	fmt.Println(valAccount[:8])
	app.valUpdates = append(app.valUpdates, val)
	Amount := binary.BigEndian.Uint64(valAccount[:8])
	account.Amount += uint32(Amount)
	app.writeAccount(account)	

	app.validatorTree.Update(addr, valAccount)
}

func (app *App) execStake(tx *Transaction) {
	account := app.execUpdate(tx)
	var err error
	
	var val abcitypes.ValidatorUpdate
	val.Power = int64(tx.Amount)
	
	val.PubKey, err = app.toPk(account.schnorrPubKey)
	if err != nil {
		panic(err)
	}
	
	addr := app.toAddress(account.schnorrPubKey)

	_, valAccount, err := app.validatorTree.Get(addr)
	if err == nil {
		val.Power += int64(binary.BigEndian.Uint64(valAccount[:8]))
	} else {
		valAccount = make([]byte, 40)
	}
	fmt.Println(valAccount[:8])
	binary.BigEndian.PutUint64(valAccount[:8], uint64(val.Power))
	
	if err == nil {
		app.validatorTree.Update(addr, valAccount)
	} else {
		copy(valAccount[8:], account.schnorrPubKey)
		app.validatorTree.Add(addr, valAccount)
	}
	
	app.valUpdates = append(app.valUpdates, val)
}

func (app *App) execCreateAccount(tx *Transaction) []byte {
	fmt.Println("Creating new account...")                            
	account := new(Account)
	account.Data = tx.data[4:88] 
	
	//find next account address
	nextaddr := app.accountNumOnDb

	account.Address = make([]byte, 4)
	binary.BigEndian.PutUint32(account.Address, uint32(nextaddr))
	app.accountNumOnDb++
	
	//change account entry
	app.writeAccount(account)
	
	tx.target = account.Address
	app.execTransfer(tx)
	
	return account.Address
}

func (app *App) execContract(tx *Transaction) []byte  {    
	
	app.execUpdate(tx)

	var key [4]byte
	copy(key[:], tx.target)
	
	contract := app.fetchContract(key)
	
	contract.Payload = tx.payload
	
	if contract.Counter >= uint64(app.contractNumOnDb) {
		app.createContract(contract, key)
	}	else	{
		contract.Counter++
		app.writeContract(contract, key)
	}
	data := append(tx.hash[:], contract.Address...)
	hash := hashData(data)
	copy(tx.hash[:], hash)
	
	return contract.Address
}

func (app *App) createContract(contract *Contract, key [4]byte) {    	
	var	nextaddr [4]byte
	nextAddr := uint32(app.contractNumOnDb + len(app.tempNewContractMap))	
	binary.BigEndian.PutUint32(nextaddr[:], nextAddr)
	contract.Address = nextaddr[:]	
	contract.counter = []byte{0, 0, 0, 0, 0, 0 ,0, 0}
	app.tempNewContractMap[nextaddr] = contract
}

func (app *App) writeContract(contract *Contract, key [4]byte) {  
	binary.BigEndian.PutUint64(contract.counter, contract.Counter)
	app.tempContractMap[key] = contract
}

func (app *App) fetchContract(key [4]byte) *Contract {    
	
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

func (app *App) execUpdate(tx *Transaction) *Account {    
	/// update source account
	// Fetch account
	account, err := app.fetchAccount(tx.source)
	if err != nil {
		//this should not happen
		return nil
	}
	
	fmt.Println("AMOUNTS: ", tx.Amount, account.Amount)
	// Subtract amount from account
	account.Amount -= (tx.Amount + app.gas * uint32(tx.length))
	fmt.Println("AMOUNTS: ", tx.Amount, account.Amount)
		
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
		return                           
        }                                     
        // Subtract amount from account                      
        account.Amount += tx.Amount                           
		            
        // Write updated account to database                 
        app.writeAccount(account)                      
}

func hashData(data []byte) ([]byte) {
	h2, err := poseidon.HashBytes(data)
	if err != nil {
		fmt.Println("POSEIDON ERROR________________________ _ _ _")
		panic(err)
	}
	
	result := arbo.BigIntToBytes(32, h2)
	return result 
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

	//reset account map	
	app.tempAccountMap = make(map[[4]byte]*Account)	
	
	//reset contract maps
	app.tempContractMap = make(map[[4]byte]*Contract)
	
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
	result := hashData(blockRoot)
	
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
		fmt.Println("BlockHashTree Error: ", err)
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

func (app *App) swapDb() error {	
	//swap and reset tx databases periodically
	if app.blockHeight % 1024 == 0 {
		//swap tx db
		app.txDbMutex.Lock()
		
		//clear one database
		err := app.clearDb(app.txStorageDb2)
		if err != nil {
			return err
		}
		
		//swap db names and references
		tempdbname := badg2
        	badg2 = badg3
        	badg3 = tempdbname         
		
        	tempdb := app.txStorageDb
        	app.txStorageDb = app.txStorageDb2                 
		app.txStorageDb2 = tempdb    
		
        	temptree := app.txStorageTree
        	app.txStorageTree = app.txStorageTree2    
		app.txStorageTree2 = temptree
		
		//reopen front db and recreate tree
		app.txStorageDb, app.txStorageTree, err = app.createTree(app.txStorageDb, 64, true)		
		if err != nil {
			return err
		}
		
		app.txDbMutex.Unlock()
		
		//swap contract db
		app.ctxDbMutex.Lock()
		
		//swap db names and references
		tempdbname = contractdb
		contractdb = contractdb2
		contractdb2 = tempdbname
		
		tempcdb := app.contractStorageDb
		app.contractStorageDb = app.contractStorageDb2
		app.contractStorageDb2 = tempcdb
		
		//reset contract db
		err = app.clearDb(app.contractStorageDb)
		if err != nil {
			return err
		}
		
		app.ctxDbMutex.Unlock()
	}
	return nil
}

func (app *App) clearDb(db *badb.BadgerDB) error {
	// Get a write transaction
	tx := db.WriteTx()

	// Iterate through all key-value pairs
	err := db.Iterate(nil, func(key, value []byte) bool {
    	// Delete the key
    	if err := tx.Delete(key); err != nil {
        	// Handle error
        		return false
	    	}
	    	return true
	})

	// Check for any errors during iteration
	if err != nil {
    		// Handle error
	    	return err
	}

	// Commit the transaction to clear the database
	err = tx.Commit()
	if err != nil {
	    	// Handle error
	    	return err
	}
	return nil
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
	rTx := app.accountLedgerDb.ReadTx()
	address, err := rTx.Get(blskey)	
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



