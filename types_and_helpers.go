package main

import (
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/wire"
	"strconv"
)

const PROFILE = false
const BLOCK_NUM_DIFF = 6

// Batch ranges =  [(1), (2), (3-4), (5-9), (10-49), (50-99), (100+)]
const BATCH_RANGE_LENGTH = 7

// Fields (don't need to be indexed) in influxdb
type BlockMetrics struct {
	AverageFee     int64 `json:"avgfee"`
	AverageFeeRate int64 `json:"avgfeerate"`
	AverageTxSize  int64 `json:"avgtxsize"`

	Hash   string `json:"blockhash"`
	Height int64  `json:"height"`
	Ins    int64  `json:"ins"`

	MaxFee     int64 `json:"maxfee"`
	MaxFeeRate int64 `json:"maxfeerate"`
	MaxTxSize  int64 `json:"maxtxsize"`

	MedianFee     int64 `json:"medianfee"`
	MedianFeeRate int64 `json:"medianfeerate"`
	MedianTime    int64 `json:"mediantime`
	MedianTxSize  int64 `json:"mediantxsize"`

	MinFee     int64 `json:"minfee"`
	MinFeeRate int64 `json:"minfeerate"`
	MinTxSize  int64 `json:"mintxsize"`

	Outs              int64 `json:"outs"`
	Subsidy           int64 `json:"subsidy"`
	SegWitTotalSize   int64 `json:"swtotal_size"`
	SegWitTotalWeight int64 `json:"swtotal_weight"`
	SegWitTxs         int64 `json:"swtxs"`

	Time        int64 `json:"time"`
	TotalOut    int64 `json:"total_out"`
	TotalSize   int64 `json:"total_size"`
	TotalWeight int64 `json:"total_weight"`
	TotalFee    int64 `json:"totalfee"`

	Txs              int64 `json:"txs"`
	UTXOIncrease     int64 `json:"utxo_increase"`
	UTXOSizeIncrease int64 `json:"utxo_size_inc"`

	// TODO: figure out how to compute without so many RPC calls...
	NumTxnsSpendingP2SH           int
	NumTxnsSpendingP2WPKH         int
	NumTxnsSpendingP2WSH          int
	NumTxnsSendingToNativeWitness int

	NumTxnsSignalingRBF    int
	NumTxnsThatConsolidate int

	// Batching metrics
	NumTxnsThatBatch int
	NumPerSizeRange  [BATCH_RANGE_LENGTH]int

	// Number of each of these output types created.
	NumP2SHOutputsCreated   int
	NumP2WSHOutputsCreated  int
	NumP2WPKHOutputsCreated int
}

// Combine the metrics learned from a single transaction into the total for the block.
func (metrics *BlockMetrics) mergeTxnMetricsDiff(diff BlockMetrics) {
	metrics.NumTxnsSpendingP2SH += diff.NumTxnsSpendingP2SH
	metrics.NumTxnsSpendingP2WPKH += diff.NumTxnsSpendingP2WPKH
	metrics.NumTxnsSpendingP2WSH += diff.NumTxnsSpendingP2WSH
	metrics.NumTxnsSendingToNativeWitness += diff.NumTxnsSendingToNativeWitness

	metrics.NumTxnsSignalingRBF += diff.NumTxnsSignalingRBF
	metrics.NumTxnsThatBatch += diff.NumTxnsThatBatch
	metrics.NumTxnsThatConsolidate += diff.NumTxnsThatConsolidate

	metrics.NumTxnsThatBatch += diff.NumTxnsThatBatch
	for i := 0; i < BATCH_RANGE_LENGTH; i++ {
		metrics.NumPerSizeRange[i] += diff.NumPerSizeRange[i]
	}

	metrics.NumP2SHOutputsCreated += diff.NumP2SHOutputsCreated
	metrics.NumP2WSHOutputsCreated += diff.NumP2WSHOutputsCreated
	metrics.NumP2WPKHOutputsCreated += diff.NumP2WPKHOutputsCreated
}

func (metrics *BlockMetrics) setBlockStats(stats *btcjson.GetBlockStatsResult) {
	metrics.AverageFee = stats.AverageFee
	metrics.AverageFeeRate = stats.AverageFeeRate
	metrics.AverageTxSize = stats.AverageTxSize

	metrics.Hash = stats.Hash
	metrics.Height = stats.Height
	metrics.Ins = stats.Ins

	metrics.MaxFee = stats.MaxFee
	metrics.MaxFeeRate = stats.MaxFeeRate
	metrics.MaxTxSize = stats.MaxTxSize

	metrics.MedianFee = stats.MedianFee
	metrics.MedianFeeRate = stats.MedianFeeRate
	metrics.MedianTime = stats.MedianTime
	metrics.MedianTxSize = stats.MedianTxSize

	metrics.MinFee = stats.MinFee
	metrics.MinFeeRate = stats.MinFeeRate
	metrics.MinTxSize = stats.MinTxSize

	metrics.Outs = stats.Outs
	metrics.Subsidy = stats.Outs
	metrics.SegWitTotalSize = stats.SegWitTotalSize
	metrics.SegWitTotalWeight = stats.SegWitTotalWeight
	metrics.SegWitTxs = stats.SegWitTxs

	metrics.Time = stats.Time
	metrics.TotalOut = stats.TotalOut
	metrics.TotalSize = stats.TotalSize
	metrics.TotalWeight = stats.TotalWeight
	metrics.TotalFee = stats.TotalFee

	metrics.Txs = stats.Txs
	metrics.UTXOIncrease = stats.UTXOIncrease
	metrics.UTXOSizeIncrease = stats.UTXOSizeIncrease
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

// Updates metrics based on the address type of this spent output.
func (metrics *BlockMetrics) setSpentOutputType(output *wire.TxOut) {
	// TODO: Check for native SegWit address type.

	if outputIsP2SH(output) {
		metrics.NumTxnsSpendingP2SH = 1
		return
	}

	if outputIsP2WSH(output) {
		metrics.NumTxnsSpendingP2WSH = 1
		return
	}

	if outputIsP2WPKH(output) {
		metrics.NumTxnsSpendingP2WPKH = 1
		return
	}
}

// Updates metrics based on the address type of this created output.
func (metrics *BlockMetrics) setCreatedOutputType(output *wire.TxOut) {
	// TODO: Check for native SegWit address type.

	if outputIsP2SH(output) {
		metrics.NumP2SHOutputsCreated += 1
		return
	}

	if outputIsP2WSH(output) {
		metrics.NumP2WSHOutputsCreated += 1
		return
	}

	if outputIsP2WPKH(output) {
		metrics.NumP2WPKHOutputsCreated += 1
		return
	}
}

// Set batch range based on number of outputs.
func setBatchRangeForTxn(txn *wire.MsgTx, metrics *BlockMetrics) {
	i := len(txn.TxOut)

	switch {
	case i == 1:
		metrics.NumPerSizeRange[0] = 1
	case i == 2:
		metrics.NumPerSizeRange[1] = 1
	case (i == 3) || (i == 4):
		metrics.NumPerSizeRange[2] = 1
	case (5 <= i) && (i <= 9):
		metrics.NumPerSizeRange[3] = 1
	case (10 <= i) && (i <= 49):
		metrics.NumPerSizeRange[4] = 1
	case (50 <= i) && (i <= 99):
		metrics.NumPerSizeRange[5] = 1
	default: // >= 100
		metrics.NumPerSizeRange[6] = 1
	}
}

func (metrics *BlockMetrics) setInfluxTags(tags map[string]string) {
	tags["height"] = strconv.Itoa(int(metrics.Height))
}

func (metrics *BlockMetrics) setInfluxFields(fields map[string]interface{}) {
	fields["avg_fee"] = metrics.AverageFee
	fields["avg_fee_rate"] = metrics.AverageFeeRate
	fields["avg_tx_size"] = metrics.AverageTxSize

	fields["max_fee"] = metrics.MaxFee
	fields["max_fee_rate"] = metrics.MaxFeeRate
	fields["max_tx_size"] = metrics.MaxTxSize

	fields["min_fee"] = metrics.MinFee
	fields["min_fee_rate"] = metrics.MinFeeRate
	fields["min_tx_size"] = metrics.MinTxSize

	fields["median_fee"] = metrics.MedianFee
	fields["median_fee_rate"] = metrics.MedianFeeRate
	fields["median_tx_size"] = metrics.MedianTxSize

	fields["block_size"] = metrics.TotalSize
	fields["volume_btc"] = metrics.TotalOut
	fields["num_txs"] = metrics.Txs

	fields["hash"] = metrics.Hash
	fields["num_inputs"] = metrics.Ins
	fields["num_outputs"] = metrics.Outs
	fields["subsidy"] = metrics.Subsidy
	fields["segwit_total_size"] = metrics.SegWitTotalSize
	fields["segwit_total_weight"] = metrics.SegWitTotalWeight
	fields["num_segwit_txs"] = metrics.SegWitTxs

	fields["total_amount_out"] = metrics.TotalOut
	fields["total_size"] = metrics.TotalSize
	fields["total_weight"] = metrics.TotalWeight
	fields["total_fee"] = metrics.TotalFee

	fields["utxo_increase"] = metrics.UTXOIncrease
	fields["utxo_size_increase"] = metrics.UTXOIncrease

	fields["frac_spending_P2SH"] = float64(metrics.NumTxnsSpendingP2SH) / float64(metrics.Txs)
	fields["frac_spending_P2WPKH"] = float64(metrics.NumTxnsSpendingP2WPKH) / float64(metrics.Txs)
	fields["frac_spending_P2WSH"] = float64(metrics.NumTxnsSpendingP2WSH) / float64(metrics.Txs)
	fields["frac_sending_to_native_witness"] = float64(metrics.NumTxnsSendingToNativeWitness) / float64(metrics.Txs)
	fields["frac_signalling_RBF"] = float64(metrics.NumTxnsSignalingRBF) / float64(metrics.Txs)
	fields["frac_batching"] = float64(metrics.NumTxnsThatBatch) / float64(metrics.Txs)
	fields["frac_consolidating"] = float64(metrics.NumTxnsThatConsolidate) / float64(metrics.Txs)
	fields["num_consolidating"] = metrics.NumTxnsThatConsolidate
	fields["num_batching"] = metrics.NumTxnsThatBatch

	// Batch ranges =  [(1), (2), (3-4), (5-9), (10-49), (50-99), (100+)]
	// TODO: name this field something more descriptive if possible.
	fields["batch_range_0"] = float64(metrics.NumPerSizeRange[0]) / float64(metrics.Txs)
	fields["batch_range_1"] = float64(metrics.NumPerSizeRange[1]) / float64(metrics.Txs)
	fields["batch_range_2"] = float64(metrics.NumPerSizeRange[2]) / float64(metrics.Txs)
	fields["batch_range_3"] = float64(metrics.NumPerSizeRange[3]) / float64(metrics.Txs)
	fields["batch_range_4"] = float64(metrics.NumPerSizeRange[4]) / float64(metrics.Txs)
	fields["batch_range_5"] = float64(metrics.NumPerSizeRange[5]) / float64(metrics.Txs)
	fields["batch_range_6"] = float64(metrics.NumPerSizeRange[6]) / float64(metrics.Txs)

	fields["num_P2SH_outputs_created"] = metrics.NumP2SHOutputsCreated
	fields["num_P2WSH_outputs_created"] = metrics.NumP2WSHOutputsCreated
	fields["num_P2WPKH_outputs_created"] = metrics.NumP2WPKHOutputsCreated
}
