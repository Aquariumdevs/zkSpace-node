package main

import (
	"os"
	badb "go.vocdoni.io/dvote/db/badgerdb"
)

// helper strings for swapping databases
var badg0 string = "badg0"
var badg1 string = "badg1"
var badg2 string = "badg2"
var badg3 string = "badg3"

var contractdb string = "contractdb"
var contractdb2 string = "contractdb2"

func (app *App) destroyDb(dbpoint *badb.BadgerDB, dbname string) error {
	// Close any existing database and delete the files
	err := dbpoint.Close()
	if err != nil {
		logs.logError("Failed to close database "+dbname+" !!!", err)
		return err
	}

	// Remove the directory and its contents
	err = os.RemoveAll(dbname)
	if err != nil {
		logs.logError("Failed to remove database "+dbname+" files!!!", err)
		return err
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
			logs.logError("ClearDb Failed to delete one entry: ", err)
			return false
		}
		return true
	})

	// Check for any errors during iteration
	if err != nil {
		logs.logError("ClearDb iteration Failed: ", err)
		return err
	}

	// Commit the transaction to clear the database
	err = tx.Commit()
	if err != nil {
		logs.logError("ClearDb Failed to commit: ", err)
		return err
	}
	return nil
}

func (app *App) swapDb() error {
	//swap and reset tx databases periodically
	if app.blockHeight%1024 == 0 {
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
			logs.logError("Failed to recreate tree: ", err)
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
			logs.logError("Failed to clear contract storage database: ", err)
			return err
		}

		app.ctxDbMutex.Unlock()
	}
	return nil
}
