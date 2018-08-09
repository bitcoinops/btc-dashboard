package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/rpcclient"

	"github.com/go-pg/pg"
	"github.com/go-pg/pg/orm"
)

/*
   Inspired by https://github.com/jhoenicke/mempool

   Some of this is basically jhoenicke's perl code in Go :)
*/

const MEASUREMENT_GRANULARITY = 60 * time.Second
const NUM_FEE_BUCKETS = 47

var FEE_BUCKET_VALUES [NUM_FEE_BUCKETS]float64

type MempoolData struct {
	Time int64 `json:"time" sql:",notnull"`

	Size          int64   `json:"size" sql:",notnull"`
	Bytes         int64   `json:"bytes" sql:",notnull"`
	MempoolMinFee float64 `json:"mempoolminfee" sql:",notnull"`

	// Diffs show the difference between this datapoint and the previous data point.
	SizeDiff          int64   `json:"size_diff" sql:",notnull"`
	BytesDiff         int64   `json:"bytes_diff" sql:",notnull"`
	MempoolMinFeeDiff float64 `json:"mempoolminfee_diff" sql:",notnull"`

	FeeBuckets []int `json:"fee_buckets" pg:",array" sql:",notnull"`
}

func getMempoolData(mempoolInfo *btcjson.GetMempoolInfoResult, t time.Time) MempoolData {
	mempoolData := MempoolData{
		Time:          t.Unix(),
		Size:          mempoolInfo.Size,
		Bytes:         mempoolInfo.Bytes,
		MempoolMinFee: mempoolInfo.MempoolMinFee,
	}

	return mempoolData
}

func (md *MempoolData) diffWithPrev(prev *MempoolData) {
	md.SizeDiff = md.Size - prev.Size
	md.BytesDiff = md.Bytes - prev.Bytes
	md.MempoolMinFeeDiff = md.MempoolMinFee - prev.MempoolMinFee
}

type MempoolDataWorker struct {
	client   *rpcclient.Client
	pgClient *pg.DB
}

func liveMempoolAnalysis() {
	FEE_BUCKET_VALUES = [NUM_FEE_BUCKETS]float64{0.0001, 1, 2, 3, 4, 5, 6, 7, 8, 10, 12, 14, 17, 20, 25, 30, 40, 50, 60, 70, 80, 100, 120, 140, 170, 200, 250, 300, 400, 500, 600, 700, 800, 1000, 1200, 1400, 1700, 2000, 2500, 3000, 4000, 5000, 6000, 7000, 8000, 10000, 2100000000000000}

	worker := setupMempoolAnalysis()
	defer worker.shutdown()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(MEASUREMENT_GRANULARITY)
	defer ticker.Stop()

	var mempoolData MempoolData
	for {
		select {
		case t := <-ticker.C:
			log.Println("Current time: ", t)
			currentTime := time.Now()

			rawMempool, err := worker.client.GetRawMempoolVerbose()
			if err != nil {
				log.Fatal(err)
			}

			mpInfo, err := worker.client.GetMempoolInfo()
			if err != nil {
				log.Fatal(err)
			}

			nextData := getMempoolData(mpInfo, currentTime)
			nextData.diffWithPrev(&mempoolData)
			nextData.assignTxsToFeeBuckets(rawMempool)

			log.Println(nextData)

			mempoolData = nextData

			err = worker.pgClient.Insert(&mempoolData)
			if err != nil {
				log.Fatal("PG database insert failed! ", err)
			}

		case <-sigs:
			return
		}
	}
}

func (md *MempoolData) assignTxsToFeeBuckets(rawMempool map[string]btcjson.GetRawMempoolVerboseResult) {
	feeBuckets := make([]int, NUM_FEE_BUCKETS)

	for _, mempoolEntry := range rawMempool {
		feeInSats := int32(mempoolEntry.Fee * (100000000))
		txFeeRate := float64(feeInSats) / float64(mempoolEntry.Size)

		// This tx is counted in both the ancestor set and descendant set.
		txSetFeeRate := float64(mempoolEntry.AncestorFees+mempoolEntry.DescendantFees-feeInSats) / float64(mempoolEntry.AncestorSize+mempoolEntry.DescendantSize-mempoolEntry.Size)
		ancestorFeeRate := float64(mempoolEntry.AncestorFees) / float64(mempoolEntry.AncestorSize)
		descendantFeeRate := float64(mempoolEntry.DescendantFees) / float64(mempoolEntry.DescendantSize)

		trueFeeRate := max(min(descendantFeeRate, txSetFeeRate), min(txFeeRate, ancestorFeeRate))

		for i := 0; i < len(FEE_BUCKET_VALUES)-1; i++ {
			if trueFeeRate >= FEE_BUCKET_VALUES[i] && trueFeeRate < FEE_BUCKET_VALUES[i+1] {
				feeBuckets[i]++
			}
		}
	}

	md.FeeBuckets = feeBuckets
}

func min(x, y float64) float64 {
	if x < y {
		return x
	}
	return y
}

func max(x, y float64) float64 {
	if x > y {
		return x
	}
	return y
}

func setupMempoolAnalysis() MempoolDataWorker {
	BITCOIND_HOST, ok := os.LookupEnv("BITCOIND_HOST")
	if !ok {
		BITCOIND_HOST = "localhost:8332"
	}

	// Connect to local bitcoin core RPC server using HTTP POST mode.
	connCfg := &rpcclient.ConnConfig{
		Host: BITCOIND_HOST,
		User: os.Getenv("BITCOIND_USERNAME"),
		Pass: os.Getenv("BITCOIND_PASSWORD"),

		HTTPPostMode: true, // Bitcoin core only supports HTTP POST mode
		DisableTLS:   true, // Bitcoin core does not provide TLS by default
	}
	// Notice the notification parameter is nil since notifications are
	// not supported in HTTP POST mode.
	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		log.Fatal(err)
	}

	DB_ADDR, ok := os.LookupEnv("DB_ADDR")
	if !ok {
		DB_ADDR = "localhost:5432"
	}

	db := pg.Connect(&pg.Options{
		Addr:     DB_ADDR,
		User:     os.Getenv("DB_USERNAME"),
		Password: os.Getenv("DB_PASSWORD"),
		Database: os.Getenv("DB"),
	})

	model := interface{}((*MempoolData)(nil))
	err = db.CreateTable(model, &orm.CreateTableOptions{
		Temp:        false,
		IfNotExists: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Println("setup table: ", err)

	// Prints out the queries created by go-pg.
	if SHOW_QUERIES {
		db.OnQueryProcessed(func(event *pg.QueryProcessedEvent) {
			query, err := event.FormattedQuery()
			if err != nil {
				log.Fatal(err)
			}

			log.Printf("%s %s", time.Since(event.StartTime), query)
		})
	}

	worker := MempoolDataWorker{
		client:   client,
		pgClient: db,
	}

	return worker
}

func (worker *MempoolDataWorker) shutdown() {
	worker.client.Shutdown()
	worker.pgClient.Close()
}
