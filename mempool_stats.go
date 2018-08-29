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

const SHOW_QUERIES_MEMPOOL = false
const MEASUREMENT_GRANULARITY = 60 * time.Second
const NUM_FEE_BUCKETS = 47
const SATOSHIS_PER_BTC = 100000000

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

	SizePerFeeBucket     []int     `json:"sizes_per_fee_bucket" pg:",array" sql:",notnull"`     // Num txs in each fee bucket
	BytesPerFeeBucket    []int     `json:"bytes_per_fee_bucket" pg:",array" sql:",notnull"`     // Sum of vbytes of txs in this bucket
	TotalFeePerFeeBucket []float64 `json:"total_fee_per_fee_bucket" pg:",array" sql:",notnull"` // Sum of fees of txs in this bucket.

	// Diffs of the above stats with the previous datapoint.
	SizePerFeeBucketDiff     []int     `json:"sizes_per_fee_bucket_diff" pg:",array" sql:",notnull"`
	BytesPerFeeBucketDiff    []int     `json:"bytes_per_fee_bucket_diff" pg:",array" sql:",notnull"`
	TotalFeePerFeeBucketDiff []float64 `json:"total_fee_per_fee_bucket_diff" pg:",array" sql:",notnull"`
}

func getMempoolData(mempoolInfo *btcjson.GetMempoolInfoResult, t time.Time) MempoolData {
	mempoolData := MempoolData{
		Time:                     t.Unix(),
		Size:                     mempoolInfo.Size,
		Bytes:                    mempoolInfo.Bytes,
		MempoolMinFee:            mempoolInfo.MempoolMinFee,
		SizePerFeeBucket:         make([]int, NUM_FEE_BUCKETS),
		BytesPerFeeBucket:        make([]int, NUM_FEE_BUCKETS),
		TotalFeePerFeeBucket:     make([]float64, NUM_FEE_BUCKETS),
		SizePerFeeBucketDiff:     make([]int, NUM_FEE_BUCKETS),
		BytesPerFeeBucketDiff:    make([]int, NUM_FEE_BUCKETS),
		TotalFeePerFeeBucketDiff: make([]float64, NUM_FEE_BUCKETS),
	}

	return mempoolData
}

func (md *MempoolData) diffWithPrev(prev *MempoolData) {
	md.SizeDiff = md.Size - prev.Size
	md.BytesDiff = md.Bytes - prev.Bytes
	md.MempoolMinFeeDiff = md.MempoolMinFee - prev.MempoolMinFee

	for i := 0; i < NUM_FEE_BUCKETS; i++ {
		md.SizePerFeeBucketDiff[i] = md.SizePerFeeBucket[i] - prev.SizePerFeeBucket[i]
		md.BytesPerFeeBucketDiff[i] = md.BytesPerFeeBucket[i] - prev.BytesPerFeeBucket[i]
		md.TotalFeePerFeeBucketDiff[i] = md.TotalFeePerFeeBucket[i] - prev.TotalFeePerFeeBucket[i]
	}
}

func (md *MempoolData) assignTxsToFeeBuckets(rawMempool map[string]btcjson.GetRawMempoolVerboseResult) {
	for _, mempoolEntry := range rawMempool {
		// Ancestor and Descendant fee include fee deltas, so for consistency this value does too.
		feeInSats := int32(mempoolEntry.Fees.ModifiedFee * SATOSHIS_PER_BTC)
		ancestorFeeInSats := int32(mempoolEntry.Fees.AncestorFee * SATOSHIS_PER_BTC)
		descendantFeeInSats := int32(mempoolEntry.Fees.DescendantFee * SATOSHIS_PER_BTC)

		txFeeRate := float64(feeInSats) / float64(mempoolEntry.Size)

		// This tx is counted in both the ancestor set and descendant set.
		txSetFeeRate := float64(ancestorFeeInSats+descendantFeeInSats-feeInSats) / float64(mempoolEntry.AncestorSize+mempoolEntry.DescendantSize-mempoolEntry.Size)
		ancestorFeeRate := float64(ancestorFeeInSats) / float64(mempoolEntry.AncestorSize)
		descendantFeeRate := float64(descendantFeeInSats) / float64(mempoolEntry.DescendantSize)

		trueFeeRate := max(min(descendantFeeRate, txSetFeeRate), min(txFeeRate, ancestorFeeRate))

		for i := 0; i < NUM_FEE_BUCKETS; i++ {
			if trueFeeRate >= FEE_BUCKET_VALUES[i] && trueFeeRate < FEE_BUCKET_VALUES[i+1] {
				md.SizePerFeeBucket[i]++
				md.BytesPerFeeBucket[i] += int(mempoolEntry.Size)
				md.TotalFeePerFeeBucket[i] += mempoolEntry.Fees.ModifiedFee
			}
		}
	}
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

type MempoolDataWorker struct {
	client   *rpcclient.Client
	pgClient *pg.DB
}

func newMempoolData() MempoolData {
	mempoolData := MempoolData{
		SizePerFeeBucket:         make([]int, NUM_FEE_BUCKETS),
		BytesPerFeeBucket:        make([]int, NUM_FEE_BUCKETS),
		TotalFeePerFeeBucket:     make([]float64, NUM_FEE_BUCKETS),
		SizePerFeeBucketDiff:     make([]int, NUM_FEE_BUCKETS),
		BytesPerFeeBucketDiff:    make([]int, NUM_FEE_BUCKETS),
		TotalFeePerFeeBucketDiff: make([]float64, NUM_FEE_BUCKETS),
	}

	return mempoolData
}

func liveMempoolAnalysis() {
	log.Println("Starting live mempool analysis")
	FEE_BUCKET_VALUES = [NUM_FEE_BUCKETS]float64{0.0001, 1, 2, 3, 4, 5, 6, 7, 8, 10, 12, 14, 17, 20, 25, 30, 40, 50, 60, 70, 80, 100, 120, 140, 170, 200, 250, 300, 400, 500, 600, 700, 800, 1000, 1200, 1400, 1700, 2000, 2500, 3000, 4000, 5000, 6000, 7000, 8000, 10000, 2100000000000000}

	//	printQueries()
	//	return

	worker := setupMempoolAnalysis()
	defer worker.shutdown()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(MEASUREMENT_GRANULARITY)
	defer ticker.Stop()

	mempoolData := newMempoolData()
	for {
		select {
		case t := <-ticker.C:
			log.Println("Logging mempool state at time: ", t)
			currentTime := time.Now()

			rawMempool, err := worker.client.GetRawMempoolVerbose()
			if err != nil {
				fatal(err)
			}

			mpInfo, err := worker.client.GetMempoolInfo()
			if err != nil {
				fatal(err)
			}

			nextData := getMempoolData(mpInfo, currentTime)
			nextData.assignTxsToFeeBuckets(rawMempool)
			nextData.diffWithPrev(&mempoolData)

			mempoolData = nextData

			err = worker.pgClient.Insert(&mempoolData)
			if err != nil {
				fatal("PG database insert failed! ", err)
			}

		case <-sigs:
			log.Println("Shutting down mempool analysis.")
			return
		}
	}
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
		fatal(err)
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
		fatal(err)
	}

	// Prints out the queries created by go-pg.
	if SHOW_QUERIES_MEMPOOL {
		db.OnQueryProcessed(func(event *pg.QueryProcessedEvent) {
			query, err := event.FormattedQuery()
			if err != nil {
				fatal(err)
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
