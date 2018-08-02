package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"time"
)

var N_WORKERS int
var DB_USED string
var BACKUP_JSON bool

const N_WORKERS_DEFAULT = 2
const DB_WAIT_TIME = 30
const BLOCK_NUM_DIFF = 6
const MAX_ATTEMPTS = 3 // max number of DB write attempts before giving up

const CURRENT_VERSION_NUMBER = 1

func main() {
	nWorkersPtr := flag.Int("workers", N_WORKERS_DEFAULT, "Number of concurrent workers.")
	startPtr := flag.Int("start", 0, "Starting blockheight.")
	endPtr := flag.Int("end", -1, "Last blockheight to analyze.")

	// Flags for different modes of operation. Default is to live analysis/back-filling.
	updateColPtr := flag.Bool("update", false, "Set to true to add a column (you need to change bits of code first)")
	insertPtr := flag.Bool("insert-json", false, "Set to true to insert .json data files into PostgreSQL")
	recoveryFlagPtr := flag.Bool("recovery", false, "Set to true to start workers on files in ./worker-progress")
	jsonPtr := flag.Bool("json", true, "Set to false to stop json logging in /db-backup")
	flag.Parse()

	// Set global variables
	N_WORKERS = *nWorkersPtr
	BACKUP_JSON = *jsonPtr

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

	if *updateColPtr {
		addColumn()
		return
	}

	if *insertPtr {
		toPostgres()
		return
	}

	if *recoveryFlagPtr {
		recoverFromFailure()
	}

	// If an end value is given, analyze that range.
	// ( the start value defaults to 0)
	if *endPtr > 0 {
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
	createDirIfNotExist(workerProgressDir)

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
	logProgressToFile(start, start, end, file)

	for i := start; i < end; i++ {
		startBlock := time.Now()
		dash.analyzeBlock(int64(i))
		log.Printf("Worker %v: Done with %v blocks total (height=%v) after %v (%v) \n", workerID, i-start+1, i, time.Since(startTime), time.Since(startBlock))

		// Only perform the write to influxDB if there hasn't been a write in the last 5 seconds.
		// And make sure to do the write before finishing.
		if !time.Now().After(lastWriteTime) && (i != end-1) {
			continue
		}

		// Write to database.
		ok := dash.commitBatchInsert()
		if !ok {
			log.Println("DB write failed!", workerID)
			return
		}

		lastWriteTime = time.Now().Add(DB_WAIT_TIME * time.Second)

		// Record progress in file, overwriting previous record.
		logProgressToFile(start, i, end, file)
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
	// Use getblockstats RPC and merge results into the metrics struct.
	blockStatsRes, err := dash.client.GetBlockStats(blockHeight, nil)
	if err != nil {
		log.Fatal(err)
	}

	blockStats := BlockStats{blockStatsRes}

	dash.batchInsert(blockStats)
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
	nBusyWorkers := 0
	doneCh := make(chan struct{}, N_WORKERS)

	i := 0 // index into files, incremented at bottom of loop.
	for i < len(files) {
		// Check if any workers are free.
		select {
		case <-doneCh:
			nBusyWorkers--
		default:
		}

		// If all workers are busy, wait and continue.
		if nBusyWorkers >= N_WORKERS {
			time.Sleep(250 * time.Millisecond)
			continue
		}

		// Assign work to a free worker.
		nBusyWorkers++

		file := files[i]
		contentsBytes, err := ioutil.ReadFile(workerProgressDir + "/" + file.Name())
		if err != nil {
			log.Fatal(err)
		}
		contents := string(contentsBytes)

		progress := parseProgress(contents)
		log.Printf("Starting recovery worker %v on range [%v, %v) at height %v\n", i, progress[0], progress[2], progress[1])
		go func(i int, file os.FileInfo) {
			analyzeBlockRange(i, progress[1], progress[2], workerProgressDir+"/"+file.Name())
			doneCh <- struct{}{}
			wg.Done()
		}(i, file)

		i++
	}
	wg.Wait()

	log.Println("Finished with Recovery.")
}

// doLiveAnalysis does an analysis of blocks as they come in live.
// In order to avoid dealing with re-org in this code-base, it should
// stay at least 6 blocks behind.
func doLiveAnalysis(height int) {
	log.Println("Starting a live analysis of the blockchain.")

	dash := setupDashboard()
	defer dash.shutdown()

	currentDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	// Create the progress directory if it doesn't already exist.
	workerProgressDir := currentDir + "/worker-progress"
	createDirIfNotExist(workerProgressDir)

	blockCount, err := dash.client.GetBlockCount()
	if err != nil {
		log.Fatal(err)
	}

	var lastAnalysisStarted int64
	if height == 0 {
		lastAnalysisStarted = blockCount - 6
	} else {
		lastAnalysisStarted = int64(height)
	}
	workFile := fmt.Sprintf("%v/live-worker_%v", workerProgressDir, lastAnalysisStarted)

	doneCh := make(chan struct{}, N_WORKERS)
	heightInRangeOfTip := (blockCount - lastAnalysisStarted) <= 6
	nBusyWorkers := 0
	for {
		// Check if any workers are free.
		select {
		case <-doneCh:
			nBusyWorkers--
		default:
		}

		if heightInRangeOfTip || (nBusyWorkers >= N_WORKERS) {
			time.Sleep(500 * time.Millisecond)
			blockCount, err = dash.client.GetBlockCount()
			if err != nil {
				log.Fatal(err)
			}
		} else {
			go func(blockHeight int64, workFile string) {
				analyzeBlockLive(blockHeight, workFile)
				doneCh <- struct{}{}
			}(lastAnalysisStarted, workFile)

			lastAnalysisStarted += 1
			workFile = fmt.Sprintf("%v/live-worker_%v", workerProgressDir, lastAnalysisStarted)
		}

		heightInRangeOfTip = (blockCount - lastAnalysisStarted) <= 6
	}
}

// analyzeBlock uses the getblockstats RPC to compute metrics of a single block.
// It then stores the results in a batchpoint in the Dashboard's influx client.
func analyzeBlockLive(blockHeight int64, workFile string) {
	dash := setupDashboard()
	defer dash.shutdown()

	start := time.Now()

	// Create file to record progress in.
	file, err := os.Create(workFile)
	defer file.Close()
	if err != nil {
		log.Fatal(err)
	}

	// Record progress in file.
	logProgressToFile(int(blockHeight), int(blockHeight), int(blockHeight), file)

	// Use getblockstats RPC and merge results into the metrics struct.
	blockStatsRes, err := dash.client.GetBlockStats(blockHeight, nil)
	if err != nil {
		log.Fatal(err)
	}

	blockStats := BlockStats{blockStatsRes}

	// Insert into database.
	ok := dash.insert(blockStats)
	if !ok {
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
