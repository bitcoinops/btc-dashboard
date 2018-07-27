package main

import (
	"github.com/btcsuite/btcd/rpcclient"

	influxClient "github.com/influxdata/influxdb/client/v2"

	"github.com/go-pg/pg"
	"github.com/go-pg/pg/orm"

	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"
)

const JSON_DIR_RELATIVE = "/db-backup"

var JSON_DIR string

// TODO: refactor all *general* methods on dashboard to collect errors from their specific implementations

// A Dashboard contains all the components necessary to make RPC calls to bitcoind, and
// to place data into influxdb.
type Dashboard struct {
	client *rpcclient.Client

	// Fields specifically for influxdb
	iClient influxClient.Client
	bp      influxClient.BatchPoints

	// Fields specifically for postgresql
	pgClient *pg.DB
	pgBatch  []DashboardData

	DB string
}

// Assumes enviroment variables: DB, DB_USERNAME, DB_PASSWORD, BITCOIND_HOST, BITCOIND_USERNAME, BITCOIND_PASSWORD, are all set.
// influxd and bitcoind should already be started.
func setupDashboard() Dashboard {
	BITCOIND_HOST, ok := os.LookupEnv("BITCOIND_HOST")
	if !ok {
		BITCOIND_HOST = "localhost:8332"
	}
	BITCOIND_USERNAME := os.Getenv("BITCOIND_USERNAME")
	BITCOIND_PASSWORD := os.Getenv("BITCOIND_PASSWORD")

	// Connect to local bitcoin core RPC server using HTTP POST mode.
	connCfg := &rpcclient.ConnConfig{
		Host:         BITCOIND_HOST,
		User:         BITCOIND_USERNAME,
		Pass:         BITCOIND_PASSWORD,
		HTTPPostMode: true, // Bitcoin core only supports HTTP POST mode
		DisableTLS:   true, // Bitcoin core does not provide TLS by default
	}
	// Notice the notification parameter is nil since notifications are
	// not supported in HTTP POST mode.
	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		log.Fatal(err)
	}

	DB_ADDR, ok := os.LookupEnv("DB_ADDR")
	if !ok {
		switch DB_USED {
		case "influxdb":
			DB_ADDR = "http://localhost:8086"
		case "postgresql":
			DB_ADDR = "http://localhost:5432"
		}
	}

	DB := os.Getenv("DB")
	DB_USERNAME := os.Getenv("DB_USERNAME")
	DB_PASSWORD := os.Getenv("DB_PASSWORD")

	var dash Dashboard
	switch DB_USED {
	case "influxdb":
		// Setup influxdb client.
		ic, err := influxClient.NewHTTPClient(influxClient.HTTPConfig{
			Addr:     DB_ADDR,
			Username: DB_USERNAME,
			Password: DB_PASSWORD,
		})
		if err != nil {
			log.Fatal(err)
		}

		// Setup influx batchpoints.
		bp, err := influxClient.NewBatchPoints(influxClient.BatchPointsConfig{
			Database: DB,
		})
		if err != nil {
			log.Fatal(err)
		}

		dash = Dashboard{
			client:  client,
			iClient: ic,
			bp:      bp,
			DB:      DB,
		}

	case "postgresql":
		// TODO: set up with SSL
		db := pg.Connect(&pg.Options{
			Addr:     DB_ADDR,
			User:     DB_USERNAME,
			Password: DB_PASSWORD,
			Database: DB, // TODO: set this up properly
		})

		model := interface{}((*DashboardData)(nil))
		err := db.CreateTable(model, &orm.CreateTableOptions{
			Temp:        false,
			IfNotExists: true,
		})
		if err != nil {
			log.Fatal(err)
		}

		dash = Dashboard{
			client:   client,
			pgClient: db,
			pgBatch:  make([]DashboardData, 0),
			DB:       DB,
		}

	default:
		log.Fatal("unimplemented DB! ", DB_USED)
	}

	// Create directory for json files.
	if BACKUP_JSON {
		currentDir, err := os.Getwd()
		if err != nil {
			log.Fatal(err)
		}

		JSON_DIR = currentDir + JSON_DIR_RELATIVE
		if _, err := os.Stat(JSON_DIR); os.IsNotExist(err) {
			log.Printf("Creating json backup directory at: %v\n", JSON_DIR)
			err := os.Mkdir(JSON_DIR, 0777)
			if err != nil {
				log.Fatal(err)
			}
		}
	}

	return dash
}

func (dash *Dashboard) shutdown() {
	dash.client.Shutdown()

	switch DB_USED {
	case "influxdb":
		dash.iClient.Close()

	case "postgresql":
		dash.pgClient.Close()
	}
}

// inserts a data from a single getblockstats call into the dashboard's DB
func (dash *Dashboard) insert(stats BlockStats) bool {
	switch DB_USED {
	case "influxdb":
		return dash.insert_influxdb(stats)

	case "postgresql":
		return dash.insert_postgresql(stats)
	default:
		log.Fatal("unimplemented DB! ", DB_USED)
	}

	return false
}

func (dash *Dashboard) insert_influxdb(stats BlockStats) bool {
	tags := make(map[string]string)        // for influxdb
	fields := make(map[string]interface{}) // for influxdb

	// Set influx tags and fields based off of the block stats computed.
	stats.setInfluxTags(tags, stats.Height)
	stats.setInfluxFields(fields)

	// Create and add new influxdb point for this block.
	blockTime := time.Unix(stats.Time, 0)
	pt, err := influxClient.NewPoint(
		"block_metrics",
		tags,
		fields,
		blockTime,
	)
	if err != nil {
		log.Fatal("Error creating new point", err)
	}

	dash.bp.AddPoint(pt)

	// Try writing the point to influxdb.
	writeSuccessful := false
	for attempts := 0; attempts <= MAX_ATTEMPTS; attempts++ {
		err := dash.iClient.Write(dash.bp)
		if err != nil {
			log.Println("DB WRITE ERR: ", err)
			log.Println("Trying DB write again...")
			time.Sleep(1 * time.Second) // Sleep to give DB a break.
			continue
		}

		log.Printf("\n\n STORED INTO INFLUXDB \n\n")

		writeSuccessful = true
		break
	}

	return writeSuccessful
}

func (dash *Dashboard) insert_postgresql(stats BlockStats) bool {
	data := stats.transformToDashboardData()

	err := dash.pgClient.Insert(&data)
	if err != nil {
		// TODO figure out if there's a more reasonable response
		log.Fatal("PG database insert failed! ", err)
	}

	log.Printf("\n\n STORED INTO POSTGRESQL \n\n")

	if BACKUP_JSON {
		dataFileName := fmt.Sprintf("%v/%v.json", JSON_DIR, stats.Height)
		dataFile, err := os.Create(dataFileName)
		if err != nil {
			fmt.Println(err)
		}

		enc := json.NewEncoder(dataFile)
		enc.Encode(data)

		dataFile.Close()
	}
	return true
}

// setup the insertion of many BlockStats (stored internally)
// uses batch insertion / bulk insertion capabilities of DB_USED whenever possible
func (dash *Dashboard) batchInsert(stats BlockStats) {
	switch DB_USED {
	case "influxdb":
		dash.batchInsert_influxdb(stats)

	case "postgresql":
		data := stats.transformToDashboardData()
		dash.pgBatch = append(dash.pgBatch, data)
	default:
		log.Fatal("unimplemented DB! ", DB_USED)
	}
}

func (dash *Dashboard) batchInsert_influxdb(stats BlockStats) {
	tags := make(map[string]string)        // for influxdb
	fields := make(map[string]interface{}) // for influxdb

	// Set influx tags and fields based off of the block stats computed.
	stats.setInfluxTags(tags, stats.Height)
	stats.setInfluxFields(fields)

	// Create and add new influxdb point for this block.
	blockTime := time.Unix(stats.Time, 0)
	pt, err := influxClient.NewPoint(
		"block_metrics",
		tags,
		fields,
		blockTime,
	)
	if err != nil {
		log.Fatal("Error creating new point", err)
	}

	dash.bp.AddPoint(pt)
}

// actually do the write of batch created
func (dash *Dashboard) commitBatchInsert() bool {
	switch DB_USED {
	case "influxdb":
		return dash.commitBatchInsert_influxdb()

	case "postgresql":
		return dash.commitBatchInsert_postgresql()
	default:
		log.Fatal("unimplemented DB! ", DB_USED)
	}

	return false
}

func (dash *Dashboard) commitBatchInsert_influxdb() bool {
	writeSuccessful := false
	for attempts := 0; attempts <= MAX_ATTEMPTS; attempts++ {
		err := dash.iClient.Write(dash.bp)
		if err != nil {
			log.Println("DB WRITE ERR: ", err)
			log.Println("Trying DB write again...")
			time.Sleep(1 * time.Second) // Sleep to give DB a break.
			continue
		}

		writeSuccessful = true
		break
	}

	if !writeSuccessful {
		return false
	}

	log.Printf("\n\n STORED INTO INFLUXDB \n\n")

	// Setup influx batchpoints.
	bp, err := influxClient.NewBatchPoints(influxClient.BatchPointsConfig{
		Database: dash.DB,
	})
	if err != nil {
		log.Fatal("Error creating new batchpoints", err)
	}

	dash.bp = bp

	return true
}

func (dash *Dashboard) commitBatchInsert_postgresql() bool {
	err := dash.pgClient.Insert(&dash.pgBatch)
	if err != nil {
		log.Fatal("PG Commit Batch insert failed! ", err)
	}

	log.Printf("\n\n STORED INTO POSTGRESQL \n\n")

	if BACKUP_JSON {
		for _, data := range dash.pgBatch {
			dataFileName := fmt.Sprintf("%v/%v.json", JSON_DIR, data.Height)
			dataFile, err := os.Create(dataFileName)
			if err != nil {
				fmt.Println(err)
			}

			enc := json.NewEncoder(dataFile)
			enc.Encode(data)

			dataFile.Close()
		}
	}

	// Reset batch.
	dash.pgBatch = make([]DashboardData, 0)

	return true
}
