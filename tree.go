package main

import (
	"github.com/vocdoni/arbo"
	"go.vocdoni.io/dvote/db"
	badb "go.vocdoni.io/dvote/db/badgerdb"
)

func (app *App) createTreeDb(dbname string, levels int, poseidon bool) (*badb.BadgerDB, *arbo.Tree, error) {
	// Create a new database
	var opts db.Options
	opts.Path = dbname
	dbpoint, err := badb.New(opts)
	if err != nil {
		logs.logError("Failed to access database "+dbname+" !!!", err)
		return nil, nil, err
	}
	return app.createTree(dbpoint, levels, poseidon)
}

func (app *App) createTree(dbpoint *badb.BadgerDB, levels int, poseidon bool) (*badb.BadgerDB, *arbo.Tree, error) {
	// Create a new tree associated with the database
	var config arbo.Config

	config = arbo.Config{
		Database:     dbpoint,
		MaxLevels:    levels,
		HashFunction: arbo.HashFunctionBlake2b}

	if poseidon {
		config.HashFunction = arbo.HashFunctionPoseidon
	}

	Tree, err := arbo.NewTree(config)

	if err != nil {
		logs.logError("Failed to create tree !!!", err)
		return dbpoint, nil, err
	}

	return dbpoint, Tree, nil
}
