package main

import (
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	influxClient "github.com/influxdata/influxdb/client/v2"
	"log"
	"reflect"
	"sync"
	"time"
)

const BLOCK_NUM_DIFF = 6

// Consts for influxdb
const (
	DB          = "btctest"
	DB_USERNAME = "marcin"
	DB_PWD      = "af181a9c33573928734a387223384b9a318ebb36"
)

type Dashboard struct {
	client  *rpcclient.Client
	iClient *influxClient.Client
}

func main() {
	// Connect to local bitcoin core RPC server using HTTP POST mode.
	connCfg := &rpcclient.ConnConfig{
		Host:         "localhost:8332",
		User:         "marcin",
		Pass:         "af337a17c853e43e6393153e8d868578789ca20a",
		HTTPPostMode: true, // Bitcoin core only supports HTTP POST mode
		DisableTLS:   true, // Bitcoin core does not provide TLS by default
	}
	// Notice the notification parameter is nil since notifications are
	// not supported in HTTP POST mode.
	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Shutdown()

	// Setup influxdb client.
	ic, err := influxClient.NewHTTPClient(influxClient.HTTPConfig{
		Addr:     "http://localhost:8086",
		Username: DB_USERNAME,
		Password: DB_PASSWORD,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer ic.Close()

	dash := Dashboard{
		client,
		ic,
	}

	analyzedCount := int64(0) // TODO: read from db

	for {
		// Get the current block count.
		blockCount, err := client.GetBlockCount()
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Block count: %d", blockCount)

		// Only do things if the block is 6 confirmations in.
		if blockCount-analyzedCount > BLOCK_NUM_DIFF {
			dash.analyzeBlock(analyzedCount)
		} else {
			time.Sleep(100 * time.Millisecond)
		}

		return
	}
}

func (dash *Dashboard) analyzeBlock(blockNum int64) {
	// Get hash of this block.
	blockHash, err := dash.client.GetBlockHash(blockNum)
	if err != nil {
		log.Fatal("Error getting block hash")
	}
	log.Println("Blockhash: ", blockHash)

	// Get contents of this block.
	block, err := dash.client.GetBlock(blockHash)
	if err != nil {
		log.Fatal("error getting block")
	}

	// Setup influx batchpoints.
	bp, err := dash.iClient.NewBatchPoints(influxClient.BatchPointsConfig{
		Database: "btctest",
	})
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup
	blockTime := block.BlockHeader.Timestamp
	for _, txn := range block.Transactions {
		wg.Add(1)
		log.Println(txn)
		log.Println(reflect.TypeOf(txn))

		/*
			go func(txn *wire.MsgTx) {
				//analyzeTxn(txn)
				wg.Done()
			}(txn)
		*/

		break
		wg.Done()
	}
	wg.Wait()
}

func analyzeTxn(txn *wire.MsgTx) {
	// Add into Grafana-compatible db:
	//// fee-rate
	// Number of inputs
	// Number of outputs
	// number of p2sh, pwpk, pw2wsh, native wtienssse,

}

// Need UTXO set breakdown too
