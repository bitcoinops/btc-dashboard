package main

import (
	"fmt"
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

	// Given one argument, test getblockstats.
	if len(os.Args) == 1 {
		blockHeight, err := strconv.Atoi(os.Args[1])
		if err != nil {
			log.Fatal("Error parsing argument, should be an int: ", err)
		}

		checkGetBlockStatsRPC(blockHeight)
		return
	}

	if len(os.Args) == 3 {
		start, err := strconv.Atoi(os.Args[1])
		if err != nil {
			log.Fatal("Error parsing first argument, should be an int: ", err)
		}

		end, err := strconv.Atoi(os.Args[2])
		if err != nil {
			log.Fatal("Error parsing second argument, should be an int: ", err)
		}

		analyze(start, end)
		return
	}

	// Given no arguments, perform the recovery process and start live analysis.
	recoverFromFailure()
	doLiveAnalysis()
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
const N_WORKERS = 18

// Splits up work across N_WORKERS workers,each with their own RPC/db clients.
func analyze(start, end int) {
	var wg sync.WaitGroup
	workSplit := (end - start) / N_WORKERS

	currentDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	// Create the progress directory if it doesn't already exist.
	workerProgressDir := currentDir + "/worker-progress"
	if _, err := os.Stat(workerProgressDir); os.IsNotExist(err) {
		log.Printf("Creating worker progress directory at: %v\n", workerProgressDir)
		err := os.Mkdir(workerProgressDir, 0777)
		if err != nil {
			log.Fatal(err)
		}
	}

	for i := 0; i < N_WORKERS; i++ {
		wg.Add(1)
		go func(i int) {
			analyzeBlockRange(i, start+(workSplit*i), start+(workSplit*(i+1)), workerProgressDir)
			wg.Done()
		}(i)
	}
	wg.Wait()
}

const DB_WAIT_TIME = 30

// Analyzes all blocks from in the interval [start, end)
func analyzeBlockRange(workerID, start, end int, dir string) {
	dash := setupDashboard()
	defer dash.shutdown()

	log.Println(start, end)

	// Keep track of time since last write.
	// If it was less than 5 seconds ago. don't write yet.
	// prevents us from overwhelming influxdb
	lastWriteTime := time.Now()
	lastWriteTime = lastWriteTime.Add(DB_WAIT_TIME * time.Second)
	startTime := time.Now()

	// Get name for this worker's progress file.
	formattedTime := lastWriteTime.Format("01-02:15:04")
	workFile := fmt.Sprintf("%v/worker-%v_%v", dir, workerID, formattedTime)

	// Create file to record progress in.
	file, err := os.Create(workFile)
	defer file.Close()
	if err != nil {
		log.Fatal(err)
	}

	// Record progress in file.
	progress := fmt.Sprintf("Start=%v\nLast=%v\nEnd=%v", start, start, end)
	_, err = file.WriteAt([]byte(progress), 0)
	if err != nil {
		log.Fatal(err)
	}

	for i := start; i < end; i++ {
		startBlock := time.Now()

		dash.analyzeBlock(int64(i))
		log.Printf("Worker %v: Done with %v blocks (height=%v) after %v \n", workerID, i-start+1, i, time.Since(startBlock))

		// Only perform the write to influxDB if there hasn't been a write in the last 5 seconds.
		// And make sure to do the write before finishing.
		if !time.Now().After(lastWriteTime) && (i != end-1) {
			continue
		}

		for {
			err := dash.iClient.Write(dash.bp)
			if err != nil {
				log.Println("DB WRITE ERR: ", err)
				log.Println("Trying DB write again...")
				time.Sleep(1 * time.Second) // Sleep to give DB a break.
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

		lastWriteTime = time.Now()
		lastWriteTime.Add(DB_WAIT_TIME * time.Second)

		// Record progress in file, overwriting previous record.
		progress := fmt.Sprintf("Start=%v\nLast=%v\nEnd=%v", start, i, end)
		_, err = file.WriteAt([]byte(progress), 0)
		if err != nil {
			log.Fatal(err)
		}
	}

	// Worker finished successfully so its progress record is unneeded.
	err = os.Remove(workFile)
	if err != nil {
		log.Printf("Error removing %v: %v\n", workFile, err)
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

// recoverFromFailure checks the worker-progress directory for any unfinished work from a previous job.
// If there is any, it starts a new worker to continue the work for each previously failed worker.
func recoverFromFailure() {
	// TODO: implement
}

// doLiveAnalysis does an analysis of blocks as they come in live.
// In order to avoid dealing with re-org in this code-base, it should
// stay at least 6 blocks behind.
func doLiveAnalysis() {
	// TODO: implement

}
