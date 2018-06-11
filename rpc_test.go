package main

import (
	"encoding/json"
	"github.com/btcsuite/btcd/rpcclient"
	"sync"
	"time"
)

const BLOCK_NUM_DIFF = 6

func main() {
	// Connect to local bitcoin core RPC server using HTTP POST mode.
	connCfg := &rpcclient.ConnConfig{
		Host:         "localhost:8332",
		User:         "yourrpcuser",
		Pass:         "yourrpcpass",
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
			analyzeBlock(analyzedCount)
		} else {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func analyzeBlock(blockNum int64) {
	// Get hash of this block.
	blockHash, err := client.GetBlockHash(blockNum)
	if err != nil {
		log.Fatal("Error getting block hash")
	}

	// Get contents of this block.
	block, err := client.GetBlock(blockHash)
	if err != nil {
		log.Fatal("error getting block")
	}

	var wg sync.WaitGroup
	for txn, _ := range block.Transactions {
		wg.Add(1)
		go analyzeTxn(txn, wg)
	}
	wg.Wait()
}

func analyzeTxn(txn *MsgTx) {
	// Add into Grafana-compatible db:
	//// fee-rate
	// Number of inputs
	// Number of outputs
	// number of p2sh, pwpk, pw2wsh, native wtienssse,

}

// Need UTXO set breakdown too
