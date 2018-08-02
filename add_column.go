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

HOW TO ADD A NEW COLUMN!

Define a primary key in the database (blockheight)

  ALTER TABLE dashboard_DATA ADD PRIMARY KEY (height);

Add column in psql

  ALTER TABLE dashboard_data ADD COLUMN mto_consolidations bigint;


Add the column to the DashboardData struct

Tweak getblockstats, and the getblockstats RPC

Example code should work for single updates (but check on local dashboard_data so you don't break things)

Then carefully test the batched updates, which may or may not be possibly with go-pg

*/

// many_to_one_consolidations

// Open file, get stats, add new stats, save updated file, do update on postgres
func (dash *Dashboard) updateColumn(fileName string) bool {
	blockHeightStr := strings.Split(fileName, ".")[0]
	blockHeight, err := strconv.Atoi(blockHeightStr)
	if err != nil {
		log.Fatal(err)
	}

	dataFileName := JSON_DIR + "/" + fileName

	blockStatsRes, err := dash.client.GetBlockStats(int64(blockHeight), &[]string{"cons_inv"})
	if err != nil {
		log.Fatal(err)
	}
	stats := BlockStats{blockStatsRes}

	file, err := os.OpenFile(dataFileName, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		log.Fatal(err, dataFileName)
	}

	var data DashboardData
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
		log.Fatal(err)
	}
	_, err = file.Seek(0, 0)
	if err != nil {
		log.Fatal(err)
	}

	// Write new data to file.
	enc := json.NewEncoder(file)
	err = enc.Encode(&data)
	if err != nil {
		log.Fatal(err, data.Height)
	}

	file.Close()
	log.Println("Done with file: ", dataFileName)

	//	res, err := dash.pgClient.Model(&data).Column("mto_consolidations").WherePK().Update()
	res, err := dash.pgClient.Model(&data).Column("mto_consolidations").Column("mto_output_count").WherePK().Returning("*").Update()
	if err != nil {
		log.Println(res)
		log.Fatal(err)
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

	files, err := ioutil.ReadDir(JSON_DIR)
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(len(files))
	doneCh := make(chan *Dashboard, N_WORKERS)

	// Fill up doneCh with free Dashboards ready to go.
	for i := 0; i < N_WORKERS; i++ {
		dash := setupDashboard()
		doneCh <- &dash
	}

	// Use available workers for work, loop finishes once all files have been assigned a worker.
	i := 0 // index into files, incremented at bottom of loop.
	for i < len(files) {
		var dash *Dashboard

		// Check if any workers are free.
		// If not, wait a little and come back.
		select {
		case freeDash := <-doneCh:
			dash = freeDash
		default:
			// Time chosen should be based on approximate time it it
			// takes for one update to complete.
			time.Sleep(150 * time.Millisecond)
			continue
		}

		fileName := files[i].Name()

		log.Println("Starting update on file: ", fileName)
		go func(fileName string, dash *Dashboard) {
			dash.updateColumn(fileName)

			doneCh <- dash
			wg.Done()
		}(fileName, dash)

		i++
	}
	wg.Wait()

	// Shutdown dashboards.
	for i := 0; i < N_WORKERS; i++ {
		dash := <-doneCh
		dash.shutdown()
	}

	log.Println("Finished with Update.")
}

// recoverFromFailure checks the worker-progress directory for any unfinished work from a previous job.
// If there is any, it starts a new worker to continue the work for each previously failed worker.
func addVersionNumbers() {
	log.Println("Starting Version Number Update Process.")
	if _, err := os.Stat(JSON_DIR); os.IsNotExist(err) {
		return
	}

	files, err := ioutil.ReadDir(JSON_DIR)
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(len(files))
	doneCh := make(chan struct{}, N_WORKERS)

	// Fill up doneCh with free Dashboards ready to go.
	for i := 0; i < N_WORKERS; i++ {
		doneCh <- struct{}{}
	}

	// Use available workers for work, loop finishes once all files have been assigned a worker.
	i := 0 // index into files, incremented at bottom of loop.
	for i < len(files) {
		// Check if any workers are free.
		// If not, wait a little and come back.
		select {
		case <-doneCh:
		default:
			// Time chosen should be based on approximate time it it
			// takes for one update to complete.
			time.Sleep(150 * time.Millisecond)
			continue
		}

		fileName := files[i].Name()

		log.Println("Starting update on file: ", fileName)
		go func(fileName string) {
			addVersionNumberToFile(fileName)
			doneCh <- struct{}{}
			wg.Done()
		}(fileName)

		i++
	}
	wg.Wait()

	log.Println("Finished with Update.")
}

const VERSION_NUMBER = 1

func addVersionNumberToFile(fileName string) bool {
	dataFileName := JSON_DIR + "/" + fileName

	file, err := os.OpenFile(dataFileName, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		log.Fatal(err, dataFileName)
	}

	var data Data
	dec := json.NewDecoder(file)
	err = dec.Decode(&data)
	if err != nil {
		log.Fatal(err)
	}

	if data.Version == VERSION_NUMBER {
		return true
	}

	data.Version = VERSION_NUMBER

	// We're going to overwrite the file with the new data value now.
	// So we clear it's contents and reset the I/O offset.
	err = file.Truncate(0)
	if err != nil {
		log.Fatal(err)
	}
	_, err = file.Seek(0, 0)
	if err != nil {
		log.Fatal(err)
	}

	// Write new data to file.
	enc := json.NewEncoder(file)
	err = enc.Encode(&data)
	if err != nil {
		log.Fatal(err)
	}

	file.Close()
	log.Println("Done with file: ", dataFileName)

	return true
}
