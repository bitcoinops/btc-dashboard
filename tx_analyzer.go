package main

import (
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	influxClient "github.com/influxdata/influxdb/client/v2"
	"log"
	"os"
	"runtime/pprof"
	"strconv"
	"sync"
	"time"
)

// A Dashboard contains all the components necessary to make RPC calls to bitcoind, and
// to place data into influxdb.
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

func main() {
	if PROFILE {
		f, _ := os.Create("cpu.out")
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	if len(os.Args) != 3 {
		log.Fatal("Expected 2 parameters: starting blockheight and ending blockheight.")
	}

	start, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatal("Error parsing first argument, should be an int: ", err)
	}

	end, err := strconv.Atoi(os.Args[2])
	if err != nil {
		log.Fatal("Error parsing second argument, should be an int: ", err)
	}

	analyze(start, end)
}

// Prints result from getblockstats RPC at the given height. Used to check local changes to getblockstats.
func checkGetBlockStatsRPC(height int) {
	dash := setupDashboard()
	defer dash.shutdown()

	// Use getblockstats RPC and merge results into the metrics struct.
	start := time.Now()
	blockStats, err := dash.client.GetBlockStats(int64(height), nil)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Blockstats: %+v\n", blockStats)
	log.Println("Time: ", time.Since(start))
}

// TODO: Try different numbers of workers
const N_WORKERS = 12

// Splits up work across N_WORKERS workers,each with their own RPC/db clients.
func analyze(start, end int) {
	var wg sync.WaitGroup
	workSplit := (end - start) / N_WORKERS

	for i := 0; i < N_WORKERS; i++ {
		wg.Add(1)
		go func(i int) {
			analyzeBlockRange(i, start+(workSplit*i), start+(workSplit*(i+1)))
			wg.Done()
		}(i)
	}
	wg.Wait()
}

// Analyzes all blocks from in the interval [start, end)
func analyzeBlockRange(workerID, start, end int) {
	dash := setupDashboard()
	defer dash.shutdown()

	log.Println(start, end)

	startTime := time.Now()
	for i := start; i < end; i++ {
		startBlock := time.Now()

		dash.analyzeBlock(int64(i))
		log.Printf("WORKER %v: Done with %v blocks after %v \n", workerID, i-start+1, time.Since(startBlock))

		// Write point to influxd.
		// Our writes are not that frequent, so there's not much point batching.
		// Some writes have been failing for unknown reasons so keep trying until it works.
		// From influxd: [monitor] 2018/06/21 12:30:40 failed to store statistics: timeout
		for {
			err := dash.iClient.Write(dash.bp)
			if err != nil {
				log.Println("DB WRITE ERR: ", err)
				log.Println("Trying DB write again...")
				continue
			}

			log.Printf("\n\n STORED INTO INFLUXDB \n\n")

			// Setup influx batchpoints.
			bp, err := influxClient.NewBatchPoints(influxClient.BatchPointsConfig{
				Database: dash.DB,
			})
			if err != nil {
				log.Fatal(err)
			}

			dash.bp = bp
			break
		}
	}

	log.Printf("Worker %v done analyzing %v blocks after %v\n", workerID, end-start, time.Since(startTime))
}

// analyzeBlock uses the getblockstats RPC to compute metrics of a single block.
// It then stores the results in a batchpoint in the Dashboard's influx client.
func (dash *Dashboard) analyzeBlock(blockHeight int64) {
	// Currently, we get the blockhash to get the block contents.
	// Block contents are used for txn analysis.
	// TODO: Move all analysis into the getblockstats RPC handler in bitcoind.
	blockHash, err := dash.client.GetBlockHash(blockHeight)
	if err != nil {
		log.Fatal("Error getting block hash", err)
	}
	block, err := dash.client.GetBlock(blockHash)
	if err != nil {
		log.Fatal("Error getting block", err)
	}

	blockMetrics := BlockMetrics{}
	tags := make(map[string]string)        // for influxdb
	fields := make(map[string]interface{}) // for influxdb

	// Use getblockstats RPC and merge results into the metrics struct.
	blockStats, err := dash.client.GetBlockStats(blockHeight, nil)
	if err != nil {
		log.Fatal(err)
	}
	blockMetrics.setBlockStats(blockStats)

	log.Printf("Blockstats: ")

	// Analyze each transaction, adding each one's contribution to the total set of metrics.
	var wg sync.WaitGroup
	diffCh := make(chan BlockMetrics, len(block.Transactions))

	// Do analysis in separate go-routines, sending results down channel.
	for _, txn := range block.Transactions {
		wg.Add(1)
		go func(txn *wire.MsgTx) {
			diff := dash.analyzeTxn(txn)
			diffCh <- diff
			wg.Done()
		}(txn)
	}

	// Close channel once all txns are done being analyzed.
	go func() {
		wg.Wait()
		close(diffCh)
	}()

	// Merge work done by txn analyzing go-routines. Loop finishes when channel is closed
	// by the goroutine created above.
	for diff := range diffCh {
		blockMetrics.mergeTxnMetricsDiff(diff)
	}

	// Set influx tags and fields based off of the blockMetrics computed.
	blockMetrics.setInfluxTags(tags)
	blockMetrics.setInfluxFields(fields)

	// Create and add new influxdb point for this block.
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

	dash.bp.AddPoint(pt)

	log.Println("Block and metrics:", blockHeight, blockMetrics)
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
