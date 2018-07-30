package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"

	"github.com/go-pg/pg"
	"github.com/go-pg/pg/orm"
)

func toPostgres() {
	/*
			   This function goes through all json files in db-dump,
		decodes the file to a DashboardData struct, and then inserts it into a postgresql table
	*/
	DB := os.Getenv("DB")
	DB_USERNAME := os.Getenv("DB_USERNAME")
	DB_PASSWORD := os.Getenv("DB_PASSWORD")
	DB_ADDR, ok := os.LookupEnv("DB_ADDR")
	if !ok {
		switch DB_USED {
		case "influxdb":
			DB_ADDR = "http://localhost:8086"
		case "postgresql":
			DB_ADDR = "http://localhost:5432"
		}
	}

	db := pg.Connect(&pg.Options{
		Addr:     DB_ADDR,
		User:     DB_USERNAME,
		Password: DB_PASSWORD,
		Database: DB, // TODO: set this up properly
	})

	defer db.Close()

	model := interface{}((*DashboardData)(nil))
	err := db.CreateTable(model, &orm.CreateTableOptions{
		Temp:        false,
		IfNotExists: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	currentDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	dataDir := currentDir + "/db-backup"
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		return
	}

	files, err := ioutil.ReadDir(dataDir)
	if err != nil {
		log.Fatal(err)
	}

	log.Println(dataDir)

	for i, fileInfo := range files {
		file, err := os.Open(dataDir + "/" + fileInfo.Name())
		if err != nil {
			log.Fatal(i, err, fileInfo.Name())
		}

		dec := json.NewDecoder(file)
		var data DashboardData

		err = dec.Decode(&data)
		if err != nil {
			log.Fatal(err, fileInfo.Name())
		}

		file.Close()

		err = db.Insert(&data)
		if err != nil {
			log.Fatal(err)
		}

		log.Println("Done with file: ", fileInfo.Name())
	}
}
