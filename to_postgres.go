package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"

	"github.com/go-pg/pg"
	"github.com/go-pg/pg/orm"
)

/*
toPostgres() goes through all json files in db-dump,
decodes the file to a their corresponding struct(s), and then inserts them into postgresql tables

If more tables are desired, you would need to add a new db.Insert here,
and define a new model using the new struct definition.
*/
func toPostgres() {
	DB_ADDR, ok := os.LookupEnv("DB_ADDR")
	if !ok {
		DB_ADDR = "http://localhost:5432"
	}

	db := pg.Connect(&pg.Options{
		Addr:     DB_ADDR,
		User:     os.Getenv("DB_USERNAME"),
		Password: os.Getenv("DB_PASSWORD"),
		Database: os.Getenv("DB"),
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

	if _, err := os.Stat(JSON_DIR); os.IsNotExist(err) {
		return
	}

	files, err := ioutil.ReadDir(JSON_DIR)
	if err != nil {
		log.Fatal(err)
	}

	for i, fileInfo := range files {
		file, err := os.Open(JSON_DIR + "/" + fileInfo.Name())
		if err != nil {
			log.Fatal(i, err, fileInfo.Name())
		}

		dec := json.NewDecoder(file)
		var data Data

		err = dec.Decode(&data)
		if err != nil {
			log.Fatal(err, fileInfo.Name())
		}

		file.Close()

		err = db.Insert(&data.DashboardDataRow)
		if err != nil {
			log.Fatal(err)
		}

		log.Println("Done with file: ", fileInfo.Name())
	}
}
