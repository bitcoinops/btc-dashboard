package main

import (
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	influxClient "github.com/influxdata/influxdb/client/v2"
	"log"
	"os"
	"runtime/pprof"
	"sync"
	"time"
)

/*

This program sets up a RPC connection with a local bitcoind instance,
and an HTTP client for a local influxdb instance.

*/

type Dashboard struct {
	client  *rpcclient.Client
	iClient influxClient.Client
	bp      influxClient.BatchPoints
	DB      string
}

// Assumes enviroment variables: DB, DB_USERNAME, DB_PASSWORD, BITCOIND_HOST, BITCOIND_USERNAME, BITCOIND_PASSWORD, are all set.
// influxd and bitcoind should already be started.
func setupDashboard() Dashboard {
	DB := os.Getenv("DB")
	DB_USERNAME := os.Getenv("DB_USERNAME")
	DB_PASSWORD := os.Getenv("DB_PASSWORD")

	BITCOIND_HOST := os.Getenv("BITCOIND_HOST")
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

	// Setup influxdb client.
	ic, err := influxClient.NewHTTPClient(influxClient.HTTPConfig{
		Addr:     "http://localhost:8086",
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

	dash := Dashboard{
		client,
		ic,
		bp,
		DB,
	}

	return dash
}

func (dash *Dashboard) shutdown() {
	dash.client.Shutdown()
	dash.iClient.Close()
}

//TODO: add arguments for start, end blocks
const START_BLOCK = 500000
const END_BLOCK = 500050

func main() {
	if PROFILE {
		f, _ := os.Create("cpu.out")
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	analysisTestTwo(START_BLOCK, END_BLOCK)
}

func (dash *Dashboard) analyzeBlock(blockHeight int64) {
	/*
		// Get hash of this block.
		blockHash, err := dash.client.GetBlockHash(blockHeight)
		if err != nil {
			log.Fatal("Error getting block hash", err)
		}

		// Get contents of this block.
		block, err := dash.client.GetBlock(blockHash)
		if err != nil {
			log.Fatal("Error getting block", err)
		}
	*/

	// Fields stored in a struct (don't need to be indexed)
	blockMetrics := BlockMetrics{}
	tags := make(map[string]string)        // for influxdb
	fields := make(map[string]interface{}) // for influxdb

	// Use getblockstats RPC and merge results into the metrics struct.
	blockStats, err := dash.client.GetBlockStats(blockHeight, nil)
	if err != nil {
		log.Fatal(err)
	}
	blockMetrics.setBlockStats(blockStats)

	/*
		// Analyze each transaction, adding each one's contribution to the total set of metrics.
		log.Println("BLOCK!!!!!!!!!!!!!!!!!!!!: ", block.Header, len(block.Transactions))
		for _, txn := range block.Transactions {
			diff := dash.analyzeTxn(txn)
			blockMetrics.mergeTxnMetricsDiff(diff)
		}
	*/

	blockMetrics.setInfluxTags(tags)
	blockMetrics.setInfluxFields(fields)

	blockTime := time.Unix(blockMetrics.Time, 0)

	pt, err := influxClient.NewPoint(
		"block_metrics",
		tags,
		fields,
		blockTime,
	)
	if err != nil {
		log.Fatal("Error creating new point", err)
	}

	log.Println("Block and metrics:", blockHeight, blockMetrics)

	dash.bp.AddPoint(pt)
}

// TODO: Implement functionality in modified getblockstats.
func (dash *Dashboard) analyzeTxn(txn *wire.MsgTx) BlockMetrics {
	const RBF_THRESHOLD = uint32(0xffffffff) - 1
	const CONSOLIDATION_MIN = 3 // Minimum number of inputs spent for it to be considered consolidation.
	const BATCHING_MIN = 3      // Minimum number of outputs for it to be considered batching.

	metricsDiff := BlockMetrics{}

	if !isCoinbaseTransaction(txn) {
		for _, input := range txn.TxIn {
			//  A transaction signals RBF any of if its input's sequence number is less than (0xffffffff - 1).

			if input.Sequence < RBF_THRESHOLD {
				metricsDiff.NumTxnsSignalingRBF = 1
				break
			}
		}
	}

	if (len(txn.TxIn) >= CONSOLIDATION_MIN) && (len(txn.TxOut) == 1) {
		metricsDiff.NumTxnsThatConsolidate = 1
	}

	// Fine-grained and general batching metrics computed.
	setBatchRangeForTxn(txn, &metricsDiff)
	if len(txn.TxOut) >= BATCHING_MIN {
		metricsDiff.NumTxnsThatBatch = 1
	}

	return metricsDiff
}

const N_WORKERS = 12

/*

Split up work amongst many workers, each with their own clients. so that they don't get bottlenecked by 1 client making RPC requests.


*/
func analysisTestTwo(start, end int) {

	var wg sync.WaitGroup
	workSplit := (END_BLOCK - START_BLOCK) / N_WORKERS
	for i := 0; i < N_WORKERS; i++ {
		wg.Add(1)
		go func(i int) {
			analyzeBlockRange(START_BLOCK+(workSplit*i), START_BLOCK+(workSplit*(i+1)))
			wg.Done()
		}(i)
	}
	wg.Wait()
}

// Analyzes all blocks from in the interval [start, end)
func analyzeBlockRange(start, end int) {
	dash := setupDashboard()
	defer dash.shutdown()

	log.Println(start, end)

	startTime := time.Now()
	for i := start; i < end; i++ {
		startBlock := time.Now()
		dash.analyzeBlock(int64(i))
		log.Println("Done with block %v after ", i, time.Since(startBlock))

		// Store points into influxdb every 1000 blocks
		if i%1000 == 0 {
			err := dash.iClient.Write(dash.bp)
			if err != nil {
				log.Println("DB WRITE ERR: ", err)
			}

			// Setup influx batchpoints.
			bp, err := influxClient.NewBatchPoints(influxClient.BatchPointsConfig{
				Database: dash.DB,
			})
			if err != nil {
				log.Fatal(err)
			}

			dash.bp = bp
		}
	}

	// Store any remaining points.
	err := dash.iClient.Write(dash.bp)
	if err != nil {
		log.Println("DB WRITE ERR: ", err)
	}

	log.Println("Time elapsed, number of blocks scanned: ", time.Since(startTime), end-start)
}
