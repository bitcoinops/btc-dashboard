package main

import (
	"github.com/btcsuite/btcd/rpcclient"

	"fmt"
	"github.com/go-pg/pg"
	"github.com/go-pg/pg/orm"
	"log"
	"os"
	"time"
)

// A Worker contains all the components necessary to make RPC calls to bitcoind, and
// to place data into PostgreSQL.
type Worker struct {
	client *rpcclient.Client

	// Fields specifically for PostgreSQL
	pgClient *pg.DB
	pgBatch  dataBatch

	workFile *os.File
}

// Assumes enviroment variables: DB, DB_USERNAME, DB_PASSWORD, BITCOIND_HOST, BITCOIND_USERNAME, BITCOIND_PASSWORD, are all set.
// PostgreSQL and bitcoind should already be started.
func setupWorker(startTime string, id int) Worker {
	workFileName := fmt.Sprintf("%v/worker-%v-%v", WORKER_PROGRESS_DIR, startTime, id)
	// Create file to record progress in.
	workFile, err := os.Create(workFileName)
	if err != nil {
		log.Fatal("Error setting up workfile: ", err)
	}

	BITCOIND_HOST, ok := os.LookupEnv("BITCOIND_HOST")
	if !ok {
		BITCOIND_HOST = "localhost:8332"
	}

	// Connect to local bitcoin core RPC server using HTTP POST mode.
	connCfg := &rpcclient.ConnConfig{
		Host: BITCOIND_HOST,
		User: os.Getenv("BITCOIND_USERNAME"),
		Pass: os.Getenv("BITCOIND_PASSWORD"),

		HTTPPostMode: true, // Bitcoin core only supports HTTP POST mode
		DisableTLS:   true, // Bitcoin core does not provide TLS by default
	}
	// Notice the notification parameter is nil since notifications are
	// not supported in HTTP POST mode.
	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		log.Fatal("Error connecting to bitcoin rpcclient", err)
	}

	DB_ADDR, ok := os.LookupEnv("DB_ADDR")
	if !ok {
		DB_ADDR = "localhost:5432"
	}

	db := pg.Connect(&pg.Options{
		Addr:     DB_ADDR,
		User:     os.Getenv("DB_USERNAME"),
		Password: os.Getenv("DB_PASSWORD"),
		Database: os.Getenv("DB"),
	})

	model := interface{}((*DashboardDataV2)(nil))
	err = db.CreateTable(model, &orm.CreateTableOptions{
		Temp:        false,
		IfNotExists: true,
	})
	if err != nil {
		log.Fatal("Error creating Postgres table: ", err)
	}

	// Prints out the queries created by go-pg.
	if SHOW_QUERIES {
		db.OnQueryProcessed(func(event *pg.QueryProcessedEvent) {
			query, err := event.FormattedQuery()
			if err != nil {
				log.Fatal("Error formatting processed query: ", err)
			}

			log.Printf("%s %s", time.Since(event.StartTime), query)
		})
	}

	worker := Worker{
		client:   client,
		pgClient: db,
		pgBatch: dataBatch{
			versions:          make([]int64, 0),
			dashboardDataRows: make([]DashboardDataV2, 0),
		},
		workFile: workFile,
	}

	return worker
}

func (worker *Worker) shutdown() {
	worker.client.Shutdown()
	worker.pgClient.Close()

	// Worker finished successfully so its progress record is unneeded.
	err := os.Remove(worker.workFile.Name())
	if err != nil {
		log.Printf("Error removing %v: %v\n", worker.workFile, err)
	}
}

// inserts a data from a single getblockstats call into the worker's DB
func (worker *Worker) insert(stats BlockStats) bool {
	data := Data{
		Version:          CURRENT_VERSION_NUMBER,
		DashboardDataRow: stats.transformToDashboardData(),
	}

	err := worker.pgClient.Insert(&data.DashboardDataRow)
	if err != nil {
		log.Fatal("PG database insert failed! ", err)
	}

	log.Printf("\n\n STORED INTO POSTGRESQL \n\n")

	if BACKUP_JSON {
		storeDataAsFile(data)
	}

	return true
}

// setup the insertion of many BlockStats (stored internally)
// uses batch insertion / bulk insertion capabilities of DB_USED whenever possible
func (worker *Worker) batchInsert(stats BlockStats) {
	worker.pgBatch.versions = append(worker.pgBatch.versions, CURRENT_VERSION_NUMBER)
	worker.pgBatch.dashboardDataRows = append(worker.pgBatch.dashboardDataRows, stats.transformToDashboardData())
}

// actually do the write of batch created
func (worker *Worker) commitBatchInsert() bool {
	err := worker.pgClient.Insert(&worker.pgBatch.dashboardDataRows)
	if err != nil {
		log.Fatal("PG Commit Batch insert failed! ", err)
	}

	log.Printf("\n\n STORED INTO POSTGRESQL \n\n")

	if BACKUP_JSON {
		for i, dashDataRow := range worker.pgBatch.dashboardDataRows {
			storeDataAsFile(Data{
				Version:          worker.pgBatch.versions[i],
				DashboardDataRow: dashDataRow,
			})
		}
	}

	// Reset batch.
	worker.pgBatch.versions = make([]int64, 0)
	worker.pgBatch.dashboardDataRows = make([]DashboardDataV2, 0)

	return true
}
