package main

import (
	"encoding/json"
	"fmt"
	"github.com/go-pg/pg"
	"log"
	"os"
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

func addColumn() {
	dash := setupDashboard()
	defer dash.shutdown()

	dash.pgClient.OnQueryProcessed(func(event *pg.QueryProcessedEvent) {
		query, err := event.FormattedQuery()
		if err != nil {
			panic(err)
		}

		log.Printf("%s %s", time.Since(event.StartTime), query)
	})

	start := 0
	end := 534701

	var wg sync.WaitGroup
	nBusyWorkers := 0
	doneCh := make(chan struct{}, N_WORKERS)

	for i := start; i < end; i++ {
		// Check if any workers are free.
		select {
		case <-doneCh:
			nBusyWorkers--
		default:
		}

		// If all workers are busy, wait and continue.
		if nBusyWorkers >= N_WORKERS {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		nBusyWorkers++
		wg.Add(1)

		go func(i int) {
			log.Println(i)
			blockStatsRes, err := dash.client.GetBlockStats(int64(i), &[]string{"cons_inv"})
			if err != nil {
				log.Fatal(err)
			}
			blockStats := BlockStats{blockStatsRes}
			dash.updateColumn(blockStats)

			wg.Done()
			doneCh <- struct{}{}
		}(i)
	}
	wg.Wait()
}

// Open file, get stats, add new stats, save updated file, do update on postgres
func (dash *Dashboard) updateColumn(stats BlockStats) bool {
	log.Println(stats)

	dataFileName := fmt.Sprintf("%v/%v.json", JSON_DIR, stats.Height)

	file, err := os.OpenFile(dataFileName, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		log.Fatal(err, dataFileName)
	}

	dec := json.NewDecoder(file)

	var data DashboardData
	err = dec.Decode(&data)
	if err != nil {
		log.Fatal("JSON decode error", err, dataFileName)
	}
	file.Close()

	log.Println(data)

	// This is essential! The height column is the only primary
	// key column, so it lets the update happen for only a specific column.
	data.Height = stats.Height
	data.Id = stats.Height

	// Set new columns only
	data.Mto_consolidations = stats.Mto_consolidations
	data.Mto_output_count = stats.Mto_output_count

	log.Println(data.Mto_output_count)
	log.Println(data)

	err = os.Remove(dataFileName)
	if err != nil {
		log.Fatal(err)
	}

	file, err = os.Create(dataFileName)
	if err != nil {
		log.Fatal(err)
	}

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
