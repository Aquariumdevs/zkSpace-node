package main

import (
	//"errors"
	"fmt"

	"github.com/fatih/color"
)

var colorReset = color.New(color.FgBlack).SprintFunc()
var colorRed = color.New(color.FgRed).SprintFunc()
var colorGreen = color.New(color.FgGreen).SprintFunc()
var colorYellow = color.New(color.FgYellow).SprintFunc()
var colorBlue = color.New(color.FgBlue).SprintFunc()

type Logger struct {
	//set this to log all internal values
	debugLogs bool
}

var logs Logger

func (logs *Logger) log(str interface{}) {
	if logs.debugLogs {
		fmt.Println(colorGreen(str))
	}
}

func (logs *Logger) logError(str string, err error) {
	fmt.Println(colorBlue(str), colorRed(err))
}

func (logs *Logger) logAccount(account *Account) {
	if logs.debugLogs {
		logs.printAccount(account)
	}
}

func (logs *Logger) printAccount(account *Account) {
	fmt.Println(colorYellow("ACCOUNT: "))
	fmt.Println("Address: ", account.Address)
	fmt.Println("Data: ", account.Data)
	fmt.Println("State: ", account.State)
	fmt.Println("schnorrPubKey: ", account.schnorrPubKey)
	//fmt.Println("blsPubKey: ", account.blsPubKey)
	//fmt.Println("pop: ", account.pop)
	fmt.Println("counter: ", account.counter)
	fmt.Println("Counter: ", account.Counter)
	fmt.Println("Amount: ", account.Amount)
	fmt.Println("Modified: ", account.Modified)
	fmt.Println("isNew: ", account.isNew)
}

func (logs *Logger) logTx(tx *Transaction) {
	if logs.debugLogs {
		logs.printTx(tx)
	}
}

func (logs *Logger) printTx(tx *Transaction) {
	fmt.Println(colorYellow("TX: "))
	fmt.Println("signature: ", tx.signature)
	fmt.Println("source: ", tx.source)
	fmt.Println("target: ", tx.target)
	fmt.Println("amount: ", tx.amount)
	fmt.Println("state: ", tx.state)
	fmt.Println("publickeys: ", tx.publickeys)
	fmt.Println("blspk: ", tx.blspk)
	fmt.Println("pop: ", tx.pop)
	fmt.Println("multisignature: ", tx.multisignature)
	fmt.Println("batchsize: ", tx.batchsize)
	fmt.Println("batch: ", tx.batch)
	fmt.Println("payload: ", tx.payload)
	fmt.Println("hash: ", tx.hash)
	fmt.Println("pubkey: ", tx.pubkey)
	fmt.Println("sourceAmount: ", tx.sourceAmount)
	fmt.Println("counter: ", tx.counter)
	fmt.Println("pad: ", tx.pad)
	fmt.Println("addresses: ", tx.addresses)
	fmt.Println("Amount: ", tx.Amount)
	fmt.Println("length: ", tx.length)
	fmt.Println("isAccountCreator: ", tx.isAccountCreator)
	fmt.Println("isAccountKeyChanger: ", tx.isAccountKeyChanger)
	fmt.Println("isContractCreator: ", tx.isContractCreator)
	fmt.Println("isContract: ", tx.isContract)
	fmt.Println("isUpdate: ", tx.isUpdate)
	fmt.Println("isTransfer: ", tx.isTransfer)
	fmt.Println("isBatch: ", tx.isBatch)
	fmt.Println("isStake: ", tx.isStake)
	fmt.Println("isDelegate: ", tx.isDelegate)
	fmt.Println("isRelease: ", tx.isRelease)
	fmt.Println("isTransfer: ", tx.isTransfer)
	fmt.Println("isCollateral: ", tx.isCollateral)
	fmt.Println("isEvidence: ", tx.isEvidence)
}
