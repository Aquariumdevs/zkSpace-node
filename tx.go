package main

type Transaction struct {
	signature      []byte
	data           []byte
	source         []byte
	target         []byte
	amount         []byte
	state          []byte
	publickeys     []byte
	blspk          []byte
	pop            []byte
	multisignature []byte
	batchsize      []byte
	batch          []byte
	payload        []byte

	hash         [32]byte
	pubkey       []byte
	sourceAmount []byte
	counter      []byte
	pad          byte

	//boolArray []bool
	addresses []byte
	//batchedTxNum int

	Amount uint32
	Fee    uint32

	length int

	isAccountCreator    bool
	isAccountKeyChanger bool
	isContractCreator   bool
	isContract          bool
	isUpdate            bool
	isTransfer          bool
	isBatch             bool
	isStake             bool
	isDelegate          bool
	isRelease           bool
	isCollateral        bool
	isEvidence          bool //this includes 2 distinct signatures on the same blockheight
	//or on the same tx counter, which indicates that the user tried
	//to confuse the system. It will impose punishment. this is
	//absolutely necessary for securing batchers of transactions
	//against spamming from malicious users
}
