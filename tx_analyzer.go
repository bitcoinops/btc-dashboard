package main

import (
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	influxClient "github.com/influxdata/influxdb/client/v2"
	"log"
	"os"
	"runtime/pprof"
	"strconv"
	"sync"
	"time"
)

/*

This program sets up a RPC connection with a local bitcoind instance,
and an HTTP client for a local influxdb instance.

TODO:
  Decide how to best figure out fee breakdowns
  Use "Another coin bites the dust" metrics for determining number of dust outputs created?

*/

const BLOCK_NUM_DIFF = 6

// TODO: fix jankness of carrying batchpoints around in this struct.
type Dashboard struct {
	client  *rpcclient.Client
	iClient influxClient.Client
	bp      influxClient.BatchPoints
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
	}

	return dash
}

func (dash *Dashboard) shutdown() {
	dash.client.Shutdown()
	dash.iClient.Close()
}

const PROFILE = false

func main() {
	if PROFILE {
		f, _ := os.Create("cpu.out")
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	analysisTestTwo()

	/*
		dash := setupDashboard()
		defer dash.shutdown()

		dash.analysisTest()
	*/
}

//TODO: add arguments for start, end blocks
const START_BLOCK = 490000
const END_BLOCK = 492000

func (dash *Dashboard) analysisTest() {

	start := time.Now()
	for i := START_BLOCK; i < END_BLOCK; i++ {
		dash.analyzeBlock(int64(i))
		/*
			// Store points into influxdb every 1000 blocks
			if i%1000 == 0 {
				err := dash.iClient.Write(dash.bp)
				if err != nil {
					log.Println("DB WRITE ERR: ", err)
				}

				// Setup influx batchpoints.
				bp, err := influxClient.NewBatchPoints(influxClient.BatchPointsConfig{
					Database: "btctest",
				})
				if err != nil {
					log.Fatal(err)
				}

				dash.bp = bp
			}
		*/
	}

	err := dash.iClient.Write(dash.bp)
	if err != nil {
		log.Println("DB WRITE ERR: ", err)
	}

	log.Println("Time elapsed: ", time.Since(start))

}

// Batch ranges =  [(1), (2), (3-4), (5-9), (10-49), (50-99), (100+)]
const BATCH_RANGE_LENGTH = 7

// Fields (don't need to be indexed) in influxdb
type BlockMetrics struct {
	blockHeight int

	totalBlockSize int //TODO: implement
	totalVolumeBTC int
	numTxns        int

	numTxnsSpendingP2SH           int
	numTxnsSpendingP2WPKH         int
	numTxnsSpendingP2WSH          int
	numTxnsSendingToNativeWitness int
	numTxnsSignalingRBF           int
	numTxnsThatConsolidate        int

	// Batching metrics
	numTxnsThatBatch int
	numPerSizeRange  [BATCH_RANGE_LENGTH]int

	//TODO: implement
	// Number of each of these output types created.
	numP2SHOutputsCreated   int
	numP2WSHOutputsCreated  int
	numP2WPKHOutputsCreated int

	// Fee statistics
	totalFee int
	// TODO: more fine-grained statistics

	// TODO: implement
	// SegWit usage metrics
	numTxnsUsingSegWit        int
	feesPaidBySegWitTxns      int
	blockSizeUsedBySegWitTxns int
	totalVolumeBySegWitTxns   int
}

// Set batch range based on number of outputs.
func setBatchRangeForTxn(txn *wire.MsgTx, metrics *BlockMetrics) {
	i := len(txn.TxOut)

	switch {
	case i == 1:
		metrics.numPerSizeRange[0] = 1
	case i == 2:
		metrics.numPerSizeRange[1] = 1
	case (i == 3) || (i == 4):
		metrics.numPerSizeRange[2] = 1
	case (5 <= i) && (i <= 9):
		metrics.numPerSizeRange[3] = 1
	case (10 <= i) && (i <= 49):
		metrics.numPerSizeRange[4] = 1
	case (50 <= i) && (i <= 99):
		metrics.numPerSizeRange[5] = 1
	default: // >= 100
		metrics.numPerSizeRange[6] = 1
	}
}

// Combine the metrics learned from a single transaction into the total for the block.
func (metrics *BlockMetrics) mergeTxnMetricsDiff(diff BlockMetrics) {
	metrics.totalVolumeBTC += diff.totalVolumeBTC
	metrics.numTxns += 1

	metrics.numTxnsSpendingP2SH += diff.numTxnsSpendingP2SH
	metrics.numTxnsSpendingP2WPKH += diff.numTxnsSpendingP2WPKH
	metrics.numTxnsSpendingP2WSH += diff.numTxnsSpendingP2WSH
	metrics.numTxnsSendingToNativeWitness += diff.numTxnsSendingToNativeWitness
	metrics.numTxnsSignalingRBF += diff.numTxnsSignalingRBF
	metrics.numTxnsThatBatch += diff.numTxnsThatBatch
	metrics.numTxnsThatConsolidate += diff.numTxnsThatConsolidate

	metrics.numTxnsThatBatch += diff.numTxnsThatBatch
	for i := 0; i < BATCH_RANGE_LENGTH; i++ {
		metrics.numPerSizeRange[i] += diff.numPerSizeRange[i]
	}

	metrics.numP2SHOutputsCreated += diff.numP2SHOutputsCreated
	metrics.numP2WSHOutputsCreated += diff.numP2WSHOutputsCreated
	metrics.numP2WPKHOutputsCreated += diff.numP2WPKHOutputsCreated

	metrics.totalFee += diff.totalFee

	metrics.numTxnsUsingSegWit += diff.numTxnsUsingSegWit
	metrics.feesPaidBySegWitTxns += diff.feesPaidBySegWitTxns
	metrics.blockSizeUsedBySegWitTxns += diff.blockSizeUsedBySegWitTxns
	metrics.totalVolumeBySegWitTxns += diff.totalVolumeBySegWitTxns
}

func (metrics *BlockMetrics) setInfluxTags(tags map[string]string) {
	tags["height"] = strconv.Itoa(metrics.blockHeight)
}

func (metrics *BlockMetrics) setInfluxFields(fields map[string]interface{}) {
	fields["block_size"] = metrics.totalBlockSize
	fields["volume_btc"] = metrics.totalVolumeBTC
	fields["num_txns"] = metrics.numTxns

	fields["frac_spending_P2SH"] = float64(metrics.numTxnsSpendingP2SH) / float64(metrics.numTxns)
	fields["frac_spending_P2WPKH"] = float64(metrics.numTxnsSpendingP2WPKH) / float64(metrics.numTxns)
	fields["frac_spending_P2WSH"] = float64(metrics.numTxnsSpendingP2WSH) / float64(metrics.numTxns)
	fields["frac_sending_to_native_witness"] = float64(metrics.numTxnsSendingToNativeWitness) / float64(metrics.numTxns)
	fields["frac_signalling_RBF"] = float64(metrics.numTxnsSignalingRBF) / float64(metrics.numTxns)
	fields["frac_batching"] = float64(metrics.numTxnsThatBatch) / float64(metrics.numTxns)
	fields["frac_consolidating"] = float64(metrics.numTxnsThatConsolidate) / float64(metrics.numTxns)

	fields["num_consolidating"] = metrics.numTxnsThatConsolidate
	fields["num_batching"] = metrics.numTxnsThatBatch

	// Batch ranges =  [(1), (2), (3-4), (5-9), (10-49), (50-99), (100+)]
	// TODO: name this field something more descriptive if possible.
	fields["batch_range_0"] = float64(metrics.numPerSizeRange[0]) / float64(metrics.numTxns)
	fields["batch_range_1"] = float64(metrics.numPerSizeRange[1]) / float64(metrics.numTxns)
	fields["batch_range_2"] = float64(metrics.numPerSizeRange[2]) / float64(metrics.numTxns)
	fields["batch_range_3"] = float64(metrics.numPerSizeRange[3]) / float64(metrics.numTxns)
	fields["batch_range_4"] = float64(metrics.numPerSizeRange[4]) / float64(metrics.numTxns)
	fields["batch_range_5"] = float64(metrics.numPerSizeRange[5]) / float64(metrics.numTxns)
	fields["batch_range_6"] = float64(metrics.numPerSizeRange[6]) / float64(metrics.numTxns)

	fields["num_P2SH_outputs_created"] = metrics.numP2SHOutputsCreated
	fields["num_P2WSH_outputs_created"] = metrics.numP2WSHOutputsCreated
	fields["num_P2WPKH_outputs_created"] = metrics.numP2WPKHOutputsCreated

	fields["sum_of_fees"] = metrics.totalFee

	fields["frac_fees_paid_by_segwit_txns"] = float64(metrics.feesPaidBySegWitTxns) / float64(metrics.totalFee)
	fields["frac_txns_using_segwit"] = float64(metrics.numTxnsUsingSegWit) / float64(metrics.numTxns)
	//fields["block_space_used_by_segwit"] = float64(metrics.blockSizeUsedBySegWitTxns) / float64(metrics.totalBlockSize) //TODO: avoid divide by 0.
	fields["volume_btc_by_segwit"] = float64(metrics.totalVolumeBySegWitTxns) / float64(metrics.totalVolumeBTC)
}

func (dash *Dashboard) analyzeBlock(blockHeight int64) {
	// Get hash of this block.
	blockHash, err := dash.client.GetBlockHash(blockHeight)
	if err != nil {
		log.Fatal("Error getting block hash", err)
	}

	// Get contents of this block.
	block, err := dash.client.GetBlock(blockHash)
	if err != nil {
		log.Fatal("Error getting block", err)
	}

	// Fields stored in a struct (don't need to be indexed)
	blockMetrics := BlockMetrics{blockHeight: int(blockHeight), numTxns: len(block.Transactions)}

	tags := make(map[string]string)
	fields := make(map[string]interface{})

	// Analyze each transaction in a separate goroutine, collect results in this thread.
	var wg sync.WaitGroup
	resultsCh := make(chan BlockMetrics, len(block.Transactions))
	for _, txn := range block.Transactions {
		wg.Add(1)

		go func(txn *wire.MsgTx) {
			metricsDiff := dash.analyzeTxn(txn)
			resultsCh <- metricsDiff
			wg.Done()

		}(txn)
	}
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Combine results as they come in from each transactions thread.
	for diff := range resultsCh {
		blockMetrics.mergeTxnMetricsDiff(diff)
	}

	blockMetrics.setInfluxTags(tags)
	blockMetrics.setInfluxFields(fields)

	pt, err := influxClient.NewPoint(
		"block_metrics",
		tags,
		fields,
		block.Header.Timestamp,
	)
	if err != nil {
		log.Fatal("Error creating new point", err)
	}

	log.Println("Block and metrics:", blockHeight, blockMetrics)

	dash.bp.AddPoint(pt)
}

func (dash *Dashboard) analyzeBlockSerial(blockHeight int64) {
	// Get hash of this block.
	blockHash, err := dash.client.GetBlockHash(blockHeight)
	if err != nil {
		log.Fatal("Error getting block hash", err)
	}

	// Get contents of this block.
	block, err := dash.client.GetBlock(blockHash)
	if err != nil {
		log.Fatal("Error getting block", err)
	}

	// Fields stored in a struct (don't need to be indexed)
	blockMetrics := BlockMetrics{blockHeight: int(blockHeight), numTxns: len(block.Transactions)}
	tags := make(map[string]string)        // for influxdb
	fields := make(map[string]interface{}) // for influxdb

	// Analyze each transaction, adding each one's contribution to the total set of metrics.

	log.Println("BLOCK!!!!!!!!!!!!!!!!!!!!: ", block.Header, len(block.Transactions))
	for _, txn := range block.Transactions {
		diff := dash.analyzeTxn(txn)
		blockMetrics.mergeTxnMetricsDiff(diff)
	}

	blockMetrics.setInfluxTags(tags)
	blockMetrics.setInfluxFields(fields)

	pt, err := influxClient.NewPoint(
		"block_metrics",
		tags,
		fields,
		block.Header.Timestamp,
	)
	if err != nil {
		log.Fatal("Error creating new point", err)
	}

	log.Println("Block and metrics:", blockHeight, blockMetrics)

	dash.bp.AddPoint(pt)
}

func (dash *Dashboard) analyzeTxn(txn *wire.MsgTx) BlockMetrics {
	const RBF_THRESHOLD = uint32(0xffffffff) - 1
	const CONSOLIDATION_MIN = 3 // Minimum number of inputs spent for it to be considered consolidation.
	const BATCHING_MIN = 3      // Minimum number of outputs for it to be considered batching.

	metricsDiff := BlockMetrics{}

	// Get value spent to compute fee.
	var totalInValue int64
	var totalOutValue int64

	if !isCoinbaseTransaction(txn) {
		for _, input := range txn.TxIn {
			//  A transaction signals RBF any of if its input's sequence number is less than (0xffffffff - 1).
			if input.Sequence < RBF_THRESHOLD {
				metricsDiff.numTxnsSignalingRBF = 1
			}

			start := time.Now()
			// Get output spent by this input.
			prevTx, err := dash.client.GetRawTransaction(&input.PreviousOutPoint.Hash)
			if err != nil {
				log.Fatal("Error getting previous output.", err, txn.TxIn[0], txn.TxHash())
			}
			log.Println("Time scanning input: ", time.Since(start))

			prevMsgTx := prevTx.MsgTx()
			spentOutput := prevMsgTx.TxOut[input.PreviousOutPoint.Index]

			// todo: combine
			// address detection into a single function.
			// TODO: track spending of native segwit outputs
			if outputIsP2SH(spentOutput) {
				metricsDiff.numTxnsSpendingP2SH = 1
				continue
			}

			if outputIsP2WSH(spentOutput) {
				metricsDiff.numTxnsSpendingP2WSH = 1
				continue
			}

			if outputIsP2WPKH(spentOutput) {
				metricsDiff.numTxnsSpendingP2WPKH = 1
				continue
			}
		}
	}

	for _, output := range txn.TxOut {
		totalOutValue += output.Value

		// TODO: Combine address detection into a single function.
		// TODO: track creation of native segwit outputs
		if outputIsP2SH(output) {
			metricsDiff.numP2SHOutputsCreated += 1
			continue
		}

		if outputIsP2WSH(output) {
			metricsDiff.numP2WSHOutputsCreated += 1
			continue
		}

		if outputIsP2WPKH(output) {
			metricsDiff.numP2WPKHOutputsCreated += 1
			continue
		}
	}

	if (len(txn.TxIn) >= CONSOLIDATION_MIN) && (len(txn.TxOut) == 1) {
		metricsDiff.numTxnsThatConsolidate = 1
	}

	// Fine-grained and rough batching statistics.
	setBatchRangeForTxn(txn, &metricsDiff)
	if len(txn.TxOut) >= BATCHING_MIN {
		metricsDiff.numTxnsThatBatch = 1
	}

	metricsDiff.totalVolumeBTC = int(totalOutValue)

	fee := totalInValue - totalOutValue
	metricsDiff.totalFee = int(fee)

	return metricsDiff
}

// TODO: Check that prevout hash is all 0.
func isCoinbaseTransaction(txn *wire.MsgTx) bool {
	const COINBASE_PREVOUT_HASH = 0x0000000000000000000000000000000000000000000000000000000000000000
	const COINBASE_PREVOUT_INDEX = 4294967295

	if len(txn.TxIn) != 1 {
		return false
	}

	prevOut := txn.TxIn[0].PreviousOutPoint

	//	if (prevOut.Hash == COINBASE_PREVOUT_HASH) && (prevOut.Index == COINBASE_PREVOUT_INDEX) {
	if prevOut.Index == COINBASE_PREVOUT_INDEX {
		return true
	} else {
		return false
	}
}

func outputIsP2SH(txOut *wire.TxOut) bool {
	const OP_HASH160 = 0xa9
	const OP_EQUAL = 0x87

	scriptPubKey := txOut.PkScript

	// Check the length.
	if len(scriptPubKey) != 23 {
		return false
	}

	if (scriptPubKey[0] != OP_HASH160) || (scriptPubKey[22] != OP_EQUAL) {
		return false
	}

	return true
}

func outputIsP2WPKH(txOut *wire.TxOut) bool {
	scriptPubKey := txOut.PkScript

	// Check the version byte and the length of the witness program.
	if (scriptPubKey[0] == 0) && (scriptPubKey[1] == 20) && len(scriptPubKey) == 22 {
		return true
	} else {
		return false
	}
}

func outputIsP2WSH(txOut *wire.TxOut) bool {
	scriptPubKey := txOut.PkScript

	// Check the version byte and the length of the witness program.
	if (scriptPubKey[0] == 0) && (scriptPubKey[1] == 32) && len(scriptPubKey) == 34 {
		return true
	} else {
		return false
	}
}

// TODO: implement.
// Updates metrics based on the address type of this spent output.
func (metrics *BlockMetrics) setSpentOutputType(txOut *wire.TxOut) {

}

// TODO: implement.
// Updates metrics based on the address type of this created output.
func (metrics *BlockMetrics) setCreatedOutputType(txOut *wire.TxOut) {

}

func (dash *Dashboard) outputDetectionTest() {
	// TODO: find P2SH, P2WPKH, P2WSH, native witness output
	// transactions to test above functions on.
}

const N_WORKERS = 8

/*

New Idea:

split up work amongst many workers, each with their own clients. so that they don't get bottlenecked by 1 client making RPC requests.


*/
func analysisTestTwo() {
	var wg sync.WaitGroup
	workSplit := (END_BLOCK - START_BLOCK) / N_WORKERS
	for i := 0; i < N_WORKERS; i++ {
		wg.Add(1)
		go func(i int) {
			analyzeBlockRange(workSplit*i, workSplit*(i+1), START_BLOCK)
			wg.Done()
		}(i)
	}
	wg.Wait()
}

func analyzeBlockRange(start, end, offset int) {
	dash := setupDashboard()
	defer dash.shutdown()

	startTime := time.Now()
	for i := start; i < end; i++ {
		dash.analyzeBlockSerial(int64(i + offset))

		// Store points into influxdb every 1000 blocks
		if i%1000 == 0 {
			err := dash.iClient.Write(dash.bp)
			if err != nil {
				log.Println("DB WRITE ERR: ", err)
			}

			// Setup influx batchpoints.
			bp, err := influxClient.NewBatchPoints(influxClient.BatchPointsConfig{
				Database: "btctest",
			})
			if err != nil {
				log.Fatal(err)
			}

			dash.bp = bp
		}
	}

	// Store any remaining points.
	err := dash.iClient.Write(dash.bp)
	if err != nil {
		log.Println("DB WRITE ERR: ", err)
	}

	log.Println("Time elapsed, number of blocks scanned: ", time.Since(startTime), end-start)
}
