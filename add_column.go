package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

/*

HOW TO ADD A NEW COLUMN/TABLE

Define a primary key in the table ("id" in the main dashboard_data table)

  ALTER TABLE dashboard_DATA ADD PRIMARY KEY (height);

Add column in psql

  ALTER TABLE dashboard_data ADD COLUMN mto_consolidations bigint;

Add the column to the DashboardDataV2 struct OR Add a new table and a corresponding struct, into Data
Tweak worker methods/fields to work with the new field.

Tweak getblockstats, and the getblockstats RPC implementation

Example code should work for single updates (but check on a test dashboard_data so you don't break things)

Then carefully test the batched updates, which may or may not be possibly with go-pg

*/

// many_to_one_consolidations update example below (actually was used)

// Open file, get stats, add new stats, save updated file, do update on postgres
func (worker *Worker) updateColumn(fileName string) bool {
	// Make dead code by default to to prevent accidental usage.
	return false

	blockHeightStr := strings.Split(fileName, ".")[0]
	blockHeight, err := strconv.Atoi(blockHeightStr)
	if err != nil {
		log.Fatal("Error getting blockHeight from file (updateColumn): ", err)
	}

	dataFileName := JSON_DIR + "/" + fileName

	blockStatsRes, err := worker.client.GetBlockStats(int64(blockHeight), &[]string{"cons_inv"})
	if err != nil {
		log.Fatal("Error with getblockstats RPC", err)
	}
	stats := BlockStats{blockStatsRes}

	file, err := os.OpenFile(dataFileName, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		log.Fatal(err, dataFileName)
	}

	var data DashboardDataV2
	dec := json.NewDecoder(file)
	err = dec.Decode(&data)
	if err != nil {
		log.Fatal("JSON decode error", err, dataFileName)
	}

	// This is essential! The height column is the only primary
	// key column, so it lets the update happen for only a specific column.
	data.Height = stats.Height
	data.Id = stats.Height

	// Set new columns only
	data.Mto_consolidations = stats.Mto_consolidations
	data.Mto_output_count = stats.Mto_output_count

	log.Println(data.Id, data.Mto_output_count)

	// We're going to overwrite the file with the new data value now.
	// So we clear it's contents and reset the I/O offset.
	err = file.Truncate(0)
	if err != nil {
		log.Fatal("File truncate error: ", err)
	}
	_, err = file.Seek(0, 0)
	if err != nil {
		log.Fatal("File seek error: ", err)
	}

	// Write new data to file.
	enc := json.NewEncoder(file)
	err = enc.Encode(&data)
	if err != nil {
		log.Fatal("JSON Encoding error: ", err, data.Height)
	}

	file.Close()
	log.Println("Done with file: ", dataFileName)

	//	res, err := worker.pgClient.Model(&data).Column("mto_consolidations").WherePK().Update()
	res, err := worker.pgClient.Model(&data).Column("mto_consolidations").Column("mto_output_count").WherePK().Returning("*").Update()
	if err != nil {
		log.Println(res)
		log.Fatal("Error updating columns", err)
	}

	return true
}

// recoverFromFailure checks the worker-progress directory for any unfinished work from a previous job.
// If there is any, it starts a new worker to continue the work for each previously failed worker.
func addColumn() {
	log.Println("Starting Column Update Process.")
	if _, err := os.Stat(JSON_DIR); os.IsNotExist(err) {
		return
	}

	formattedTime := time.Now().Format("01-02:15:04")

	files, err := ioutil.ReadDir(JSON_DIR)
	if err != nil {
		log.Fatal("Error reading directory (addColumn)", err)
	}

	var wg sync.WaitGroup
	wg.Add(len(files))
	workers := make(chan *Worker, N_WORKERS)

	// Fill up doneCh with free Workers ready to go.
	for i := 0; i < N_WORKERS; i++ {
		worker := setupWorker(formattedTime, i)
		workers <- &worker
	}

	// Use available workers for work, loop finishes once all files have been assigned a worker.
	i := 0 // index into files, incremented at bottom of loop.
	for i < len(files) {
		var worker *Worker

		// Check if any workers are free.
		// If not, wait a little and come back.
		select {
		case freeWorker := <-workers:
			worker = freeWorker
		default:
			// Time chosen should be based on approximate time it it
			// takes for one update to complete.
			time.Sleep(150 * time.Millisecond)
			continue
		}

		fileName := files[i].Name()

		log.Println("Starting update on file: ", fileName)
		go func(fileName string, worker *Worker) {
			worker.updateColumn(fileName)

			workers <- worker
			wg.Done()
		}(fileName, worker)

		i++
	}
	wg.Wait()

	// Shutdown workers.
	for i := 0; i < N_WORKERS; i++ {
		worker := <-workers
		worker.shutdown()
	}

	log.Println("Finished with Update.")
}
