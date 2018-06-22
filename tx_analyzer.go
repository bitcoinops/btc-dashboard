package main

import (
	"github.com/btcsuite/btcd/rpcclient"
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
		log.Printf("WORKER %v: Done with %v blocks (height=%v) after %v \n", workerID, i-start+1, i, time.Since(startBlock))

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

	log.Printf("Worker %v done analyzing %v blocks (height=%v) after %v\n", workerID, end-start, end, time.Since(startTime))
}

// analyzeBlock uses the getblockstats RPC to compute metrics of a single block.
// It then stores the results in a batchpoint in the Dashboard's influx client.
func (dash *Dashboard) analyzeBlock(blockHeight int64) {
	tags := make(map[string]string)        // for influxdb
	fields := make(map[string]interface{}) // for influxdb

	// Use getblockstats RPC and merge results into the metrics struct.
	blockStatsRes, err := dash.client.GetBlockStats(blockHeight, nil)
	if err != nil {
		log.Fatal(err)
	}

	blockStats := BlockStats{blockStatsRes}

	// Set influx tags and fields based off of the block stats computed.
	blockStats.setInfluxTags(tags, blockHeight)
	blockStats.setInfluxFields(fields)

	// Create and add new influxdb point for this block.
	blockTime := time.Unix(blockStats.Time, 0)
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
