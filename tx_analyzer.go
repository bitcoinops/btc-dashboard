package main

import (
	"flag"
	"fmt"
	"github.com/btcsuite/btcd/rpcclient"
	influxClient "github.com/influxdata/influxdb/client/v2"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var N_WORKERS int

const N_WORKERS_DEFAULT = 2
const DB_WAIT_TIME = 30

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
	DB_ADDR, ok := os.LookupEnv("DB_ADDR")
	if !ok {
		DB_ADDR = "http://localhost:8086"
	}

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
	recoveryFlagPtr := flag.Bool("recovery", false, "Set to true to start workers on files in ./worker-progress")
	startPtr := flag.Int("start", 0, "Starting blockheight.")
	endPtr := flag.Int("end", 0, "Last blockheight to analyze.")
	nWorkersPtr := flag.Int("workers", N_WORKERS_DEFAULT, "Number of concurrent RPC workers.")
	flag.Parse()

	N_WORKERS = *nWorkersPtr

	if *recoveryFlagPtr {
		recoverFromFailure()
	}

	// If both a start and end are given, analyze that range.
	if (*startPtr > 0) && (*endPtr > 0) {
		analyze(*startPtr, *endPtr)
		return
	}

	// Given no arguments, start live analysis.
	doLiveAnalysis(*startPtr)
}

// Splits up work across N_WORKERS workers,each with their own RPC/db clients.
func analyze(start, end int) {
	var wg sync.WaitGroup
	workSplit := (end - start) / N_WORKERS
	log.Println("work split: ", workSplit, end-start, start, end)

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

	formattedTime := time.Now().Format("01-02:15:04")

	for i := 0; i < N_WORKERS; i++ {
		wg.Add(1)
		go func(i int) {
			// Get name for this worker's progress file.
			workFile := fmt.Sprintf("%v/worker-%v_%v", workerProgressDir, i, formattedTime)

			analyzeBlockRange(i, start+(workSplit*i), start+(workSplit*(i+1)), workFile)
			wg.Done()
		}(i)
	}
	wg.Wait()
}

// Analyzes all blocks from in the interval [start, end)
func analyzeBlockRange(workerID, start, end int, workFile string) {
	dash := setupDashboard()
	defer dash.shutdown()

	log.Println(start, end)

	// Keep track of time since last write.
	// If it was less than DB_WAIT_TIME seconds ago. don't write yet.
	// prevents us from overwhelming influxdb
	lastWriteTime := time.Now()
	lastWriteTime = lastWriteTime.Add(DB_WAIT_TIME * time.Second)
	startTime := time.Now()

	// Create file to record progress in.
	// If the file already exists, this truncates it which is ok.
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
		log.Printf("Worker %v: Done with %v blocks total (height=%v) after %v (%v) \n", workerID, i-start+1, i, time.Since(startTime), time.Since(startBlock))

		// Only perform the write to influxDB if there hasn't been a write in the last 5 seconds.
		// And make sure to do the write before finishing.
		if !time.Now().After(lastWriteTime) && (i != end-1) {
			continue
		}

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
			log.Printf("DB write failed!")
			return
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
		lastWriteTime = time.Now().Add(DB_WAIT_TIME * time.Second)

		// Record progress in file, overwriting previous record.
		progress = fmt.Sprintf("Start=%v\nLast=%v\nEnd=%v", start, i, end)
		_, err = file.WriteAt([]byte(progress), 0)
		if err != nil {
			log.Fatal("Error writing progress: ", err, progress)
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
	log.Println("Starting Recovery Process.")
	currentDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	// If there is no worker-progress directory, then there aren't any failures :)
	workerProgressDir := currentDir + "/worker-progress"
	if _, err := os.Stat(workerProgressDir); os.IsNotExist(err) {
		return
	}

	files, err := ioutil.ReadDir(workerProgressDir)
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(len(files))
	nWorkersBusy := 0
	doneCh := make(chan struct{}, N_WORKERS)

	i := 0 // index into files, incremented at bottom of loop.
	for i < len(files) {
		// Check if any workers are free.
		select {
		case <-doneCh:
			nWorkersBusy--
		default:
		}

		// If all workers are busy, wait and continue.
		if nWorkersBusy >= N_WORKERS {
			time.Sleep(250 * time.Millisecond)
			continue
		}

		// Assign work to a free worker.
		nWorkersBusy++

		file := files[i]
		contentsBytes, err := ioutil.ReadFile(workerProgressDir + "/" + file.Name())
		if err != nil {
			log.Fatal(err)
		}
		contents := string(contentsBytes)

		progress := parseProgress(contents)
		if len(progress) == 3 {
			log.Printf("Starting recovery worker %v on range [%v, %v) at height %v\n", i, progress[0], progress[2], progress[1])
			go func(i int, file os.FileInfo) {
				analyzeBlockRange(i, progress[1], progress[2], workerProgressDir+"/"+file.Name())
				doneCh <- struct{}{}
				wg.Done()
			}(i, file)
		} else if len(progress) == 1 {
			// Finish work done during a live analysis.
			log.Printf("Starting recovery worker %v on block %v\n", i, progress[0])
			go func(i int, file os.FileInfo) {
				analyzeBlockLive(int64(progress[0]), workerProgressDir+"/"+file.Name())
				doneCh <- struct{}{}
				wg.Done()
			}(i, file)
		} else {
			log.Fatal("Bad progress given: ", progress)
		}

		i++
	}
	wg.Wait()

	log.Println("Finished with Recovery.")
}

// parseProgress takes in the contents of a worker-progress file
// and returns the starting height, the last height completed, and the end height.
func parseProgress(contents string) []int {
	lines := strings.Split(contents, "\n")
	result := make([]int, 0)

	for _, line := range lines {
		split := strings.Split(line, "=")

		if len(split) < 2 {
			continue
		}
		height, err := strconv.Atoi(split[1])
		if err != nil {
			log.Fatal(err)
		}

		result = append(result, height)
	}

	return result
}

// doLiveAnalysis does an analysis of blocks as they come in live.
// In order to avoid dealing with re-org in this code-base, it should
// stay at least 6 blocks behind.
func doLiveAnalysis(height int) {
	log.Println("Starting a live analysis of the blockchain.")
	formattedTime := time.Now().Format("01-02:15:04")

	dash := setupDashboard()
	defer dash.shutdown()

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

	blockCount, err := dash.client.GetBlockCount()
	if err != nil {
		log.Fatal(err)
	}

	workFile := fmt.Sprintf("%v/live-worker_%v", workerProgressDir, formattedTime)

	var lastAnalysisStarted int64
	if height == 0 {
		lastAnalysisStarted = blockCount - 6
	} else {
		lastAnalysisStarted = int64(height)
	}
	heightInRangeOfTip := (blockCount - lastAnalysisStarted) <= 6
	for {
		if heightInRangeOfTip {
			time.Sleep(500 * time.Millisecond)
			blockCount, err = dash.client.GetBlockCount()
			if err != nil {
				log.Fatal(err)
			}
		} else {
			analyzeBlockLive(lastAnalysisStarted, workFile)
			lastAnalysisStarted += 1
		}

		heightInRangeOfTip = (blockCount - lastAnalysisStarted) <= 6
	}
}

// analyzeBlock uses the getblockstats RPC to compute metrics of a single block.
// It then stores the results in a batchpoint in the Dashboard's influx client.
func analyzeBlockLive(blockHeight int64, workFile string) {
	dash := setupDashboard()
	defer dash.shutdown()

	tags := make(map[string]string)        // for influxdb
	fields := make(map[string]interface{}) // for influxdb

	start := time.Now()

	// Create file to record progress in.
	file, err := os.Create(workFile)
	defer file.Close()
	if err != nil {
		log.Fatal(err)
	}

	// Record progress in file.
	progress := fmt.Sprintf("Height=%v", blockHeight)
	_, err = file.WriteAt([]byte(progress), 0)
	if err != nil {
		log.Fatal(err)
	}

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

	if !writeSuccessful {
		log.Printf("DB write failed!")
		return
	}

	// Worker finished successfully so its progress record is unneeded.
	err = os.Remove(workFile)
	if err != nil {
		log.Printf("Error removing %v: %v\n", workFile, err)
	}

	log.Printf("Done with block %v after %v\n", blockHeight, time.Since(start))
}
