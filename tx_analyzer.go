package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"time"
)

var N_WORKERS int
var DB_USED string
var BACKUP_JSON bool
var JSON_DIR string
var WORKER_PROGRESS_DIR string

const SHOW_QUERIES = false
const JSON_DIR_RELATIVE = "/db-backup"
const WORKER_PROGRESS_DIR_RELATIVE = "/worker-progress"

const N_WORKERS_DEFAULT = 2
const DB_WAIT_TIME = 30
const MIN_DIST_FROM_TIP = 6
const MAX_ATTEMPTS = 3 // max number of DB write attempts before giving up

const CURRENT_VERSION_NUMBER = 2

func main() {
	nWorkersPtr := flag.Int("workers", N_WORKERS_DEFAULT, "Number of concurrent workers.")
	startPtr := flag.Int("start", 0, "Starting blockheight.")
	endPtr := flag.Int("end", -1, "Last blockheight to analyze.")

	// Flags for different modes of operation. Default is to live analysis/back-filling.
	mempoolPtr := flag.Bool("mempool", false, "Set to true to start a mempool analysis")
	updateColPtr := flag.Bool("update", false, "Set to true to add a column (you need to change bits of code first)")
	insertPtr := flag.Bool("insert-json", false, "Set to true to insert .json data files into PostgreSQL")
	recoveryFlagPtr := flag.Bool("recovery", false, "Set to true to start workers on files in ./worker-progress")
	jsonPtr := flag.Bool("json", true, "Set to false to stop json logging in /db-backup")
	flag.Parse()

	// Set global variables
	N_WORKERS = *nWorkersPtr
	BACKUP_JSON = *jsonPtr

	currentDir, err := os.Getwd()
	if err != nil {
		log.Fatal("Error getting working directory: ", err)
	}

	// Create directory for json files.
	if BACKUP_JSON {
		JSON_DIR = currentDir + JSON_DIR_RELATIVE
		createDirIfNotExist(JSON_DIR)
	}

	// Create worker progress directory if it doesn't exist yet.
	WORKER_PROGRESS_DIR = currentDir + WORKER_PROGRESS_DIR_RELATIVE
	if _, err := os.Stat(WORKER_PROGRESS_DIR); os.IsNotExist(err) {
		createDirIfNotExist(WORKER_PROGRESS_DIR)
	}

	if *mempoolPtr {
		liveMempoolAnalysis()
		return
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
		startBackfill(*startPtr, *endPtr)
		return
	}

	// Given no arguments, start live analysis.
	doLiveAnalysis(*startPtr)
}

// Splits up work across N_WORKERS workers,each with their own RPC/db clients.
func startBackfill(start, end int) {
	var wg sync.WaitGroup
	workSplit := (end - start) / N_WORKERS
	log.Println("Work split: ", workSplit, end-start, start, end)

	formattedTime := time.Now().Format("01-02:15:04")
	for i := 0; i < N_WORKERS; i++ {
		wg.Add(1)
		go func(i int) {
			analyzeBlockRange(formattedTime, i, start+(workSplit*i), start+(workSplit*(i+1)))
			wg.Done()
		}(i)
	}
	wg.Wait()
}

// Analyzes all blocks from in the interval [start, end)
func analyzeBlockRange(formattedTime string, workerID, start, end int) {
	worker := setupWorker(formattedTime, workerID)
	defer worker.shutdown()

	// Keep track of time since last write.
	// If it was less than DB_WAIT_TIME seconds ago. don't write yet.
	// prevents us from overwhelming the database.
	lastWriteTime := time.Now()
	lastWriteTime = lastWriteTime.Add(DB_WAIT_TIME * time.Second)

	startTime := time.Now()

	// Record progress in file.
	logProgressToFile(start, start, end, worker.workFile)

	for i := start; i < end; i++ {
		startBlock := time.Now()
		worker.analyzeBlock(int64(i))
		log.Printf("Worker %v: Done with %v blocks total (height=%v) after %v (%v) \n", workerID, i-start+1, i, time.Since(startTime), time.Since(startBlock))

		// Only perform the write to database if there hasn't been a write in the last 5 seconds.
		// And make sure to do the write before finishing.
		if !time.Now().After(lastWriteTime) && (i != end-1) {
			continue
		}

		// Write to database.
		ok := worker.commitBatchInsert()
		if !ok {
			log.Println("DB write failed!", workerID)
			return
		}

		lastWriteTime = time.Now().Add(DB_WAIT_TIME * time.Second)

		// Record progress in file, overwriting previous record.
		logProgressToFile(start, i+1, end, worker.workFile)
	}

	log.Printf("Worker %v done analyzing %v blocks (height=%v) after %v\n", workerID, end-start, end, time.Since(startTime))
}

// analyzeBlock uses the getblockstats RPC to compute metrics of a single block.
// It then stores the results in a batch to be inserted to db later.
func (worker *Worker) analyzeBlock(blockHeight int64) {
	// Use getblockstats RPC and merge results into the metrics struct.
	blockStatsRes, err := worker.client.GetBlockStats(blockHeight, nil)
	if err != nil {
		log.Fatal("Error with getblockstats RPC: ", err)
	}

	blockStats := BlockStats{blockStatsRes}

	worker.batchInsert(blockStats)
}

// recoverFromFailure checks the worker-progress directory for any unfinished work from a previous job.
// If there is any, it starts a new worker to continue the work for each previously failed worker.
func recoverFromFailure() {
	log.Println("Starting Recovery Process.")

	files, err := ioutil.ReadDir(WORKER_PROGRESS_DIR)
	if err != nil {
		log.Fatal("Error reading worker_progress directory: ", err)
	}

	var wg sync.WaitGroup
	wg.Add(len(files))

	workers := make(chan struct{}, N_WORKERS)
	for i := 0; i < N_WORKERS; i++ { // Signal that there are available workers.
		workers <- struct{}{}
	}

	progressFiles := make([]os.FileInfo, len(files))
	for i := 0; i < len(files); i++ {
		progressFiles[i] = files[i]
	}

	i := 0 // index into files, incremented at bottom of loop.
	for i < len(files) {
		// Check if any workers are free.
		select {
		case <-workers:
		default:
			// If all workers are busy, wait and continue.
			time.Sleep(1000 * time.Millisecond)
			continue
		}

		file := progressFiles[i]
		contentsBytes, err := ioutil.ReadFile(WORKER_PROGRESS_DIR + "/" + file.Name())
		if err != nil {
			log.Fatal("Error reading wp file: ", err)
		}
		contents := string(contentsBytes)
		progress := parseProgress(contents)
		log.Printf("Starting recovery worker %v on range [%v, %v) at height %v\n", i, progress[0], progress[2], progress[1])
		go func(i int) {
			analyzeBlockRange(time.Now().Format("01-02:15:04"), i, progress[1], progress[2])
			workers <- struct{}{}
			wg.Done()
		}(i)

		err = os.Remove(WORKER_PROGRESS_DIR + "/" + file.Name())
		if err != nil {
			log.Fatal("Error removing file: ", err)
		}

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

	worker := setupWorker(time.Now().Format("01-02:15:04"), height)
	defer worker.shutdown()

	blockCount, err := worker.client.GetBlockCount()
	if err != nil {
		log.Fatal("Error with getblockcount RPC: ", err)
	}

	var lastAnalysisStarted int64
	if height == 0 {
		lastAnalysisStarted = blockCount - MIN_DIST_FROM_TIP
	} else {
		lastAnalysisStarted = int64(height)
	}

	workers := make(chan struct{}, N_WORKERS)
	for i := 0; i < N_WORKERS; i++ {
		workers <- struct{}{}
	}

	heightInRangeOfTip := (blockCount - lastAnalysisStarted) <= MIN_DIST_FROM_TIP
	for {
		// Check if any workers are free.
		select {
		case <-workers:
		default:
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if heightInRangeOfTip {
			time.Sleep(500 * time.Millisecond)
			blockCount, err = worker.client.GetBlockCount()
			if err != nil {
				log.Fatal("Error with getblockcount RPC: ", err)
			}
		} else {
			go func(blockHeight int64) {
				analyzeBlockLive(blockHeight)
				workers <- struct{}{}
			}(lastAnalysisStarted)

			lastAnalysisStarted += 1
		}

		heightInRangeOfTip = (blockCount - lastAnalysisStarted) <= MIN_DIST_FROM_TIP
	}
}

// analyzeBlock uses the getblockstats RPC to compute metrics of a single block.
// It then stores the results in a database (and json file if desired).
func analyzeBlockLive(blockHeight int64) {
	worker := setupWorker(time.Now().Format("01-02:15:04"), int(blockHeight))
	defer worker.shutdown()

	start := time.Now()

	// Record progress in file.
	logProgressToFile(int(blockHeight), int(blockHeight), int(blockHeight), worker.workFile)

	// Use getblockstats RPC and merge results into the metrics struct.
	blockStatsRes, err := worker.client.GetBlockStats(blockHeight, nil)
	if err != nil {
		log.Fatal("Error with getblockstats RPC: ", err)
	}
	blockStats := BlockStats{blockStatsRes}

	// Insert into database.
	ok := worker.insert(blockStats)
	if !ok {
		log.Printf("DB write failed!")
		return
	}

	log.Printf("Done with block %v after %v\n", blockHeight, time.Since(start))
}
