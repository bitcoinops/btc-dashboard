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

	NestedP2WPKHOutputsSpent int64 `json:"nested_p2wpkh_outputs_spent"`
	NestedP2WSHOutputsSpent  int64 `json:"nested_p2wsh_outputs_spent"`
	NativeP2WPKHOutputsSpent int64 `json:"native_p2wpkh_outputs_spent"`
	NativeP2WSHOutputsSpent  int64 `json:"native_p2wsh_outputs_spent"`

	TxsSpendingNestedP2WPKHOutputs int64 `json:"txs_spending_nested_p2wpkh_outputs"`
	TxsSpendingNestedP2WSHOutputs  int64 `json:"txs_spending_nested_p2wsh_outputs"`
	TxsSpendingNativeP2WPKHOutputs int64 `json:"txs_spending_native_p2wpkh_outputs"`
	TxsSpendingNativeP2WSHOutputs  int64 `json:"txs_spending_native_p2wsh_outputs"`

	NewP2WPKHOutputs int64 `json:"new_p2wpkh_outputs"`
	NewP2WSHOutputs  int64 `json:"new_p2wsh_outputs"`

	TxsCreatingP2WPKHOutputs int64 `json:"txs_creating_p2wpkh_outputs"`
	TxsCreatingP2WSHOutputs  int64 `json:"txs_creating_p2wsh_outputs"n`

	NumTxnsSignalingRBF    int
	NumTxnsThatConsolidate int

	// Batching metrics
	NumTxnsThatBatch int
	NumPerSizeRange  [BATCH_RANGE_LENGTH]int

	DustBins0 int64 `json:"dust_bins[0]"`
	DustBins1 int64 `json:"dust_bins[1]"`
	DustBins2 int64 `json:"dust_bins[2]"`
	DustBins3 int64 `json:"dust_bins[3]"`
	DustBins4 int64 `json:"dust_bins[4]"`
	DustBins5 int64 `json:"dust_bins[5]"`
	DustBins6 int64 `json:"dust_bins[6]"`
	DustBins7 int64 `json:"dust_bins[7]"`
	DustBins8 int64 `json:"dust_bins[8]"`
}

// Combine the metrics learned from a single transaction into the total for the block.
func (metrics *BlockMetrics) mergeTxnMetricsDiff(diff BlockMetrics) {
	metrics.NumTxnsSignalingRBF += diff.NumTxnsSignalingRBF
	metrics.NumTxnsThatBatch += diff.NumTxnsThatBatch
	metrics.NumTxnsThatConsolidate += diff.NumTxnsThatConsolidate

	metrics.NumTxnsThatBatch += diff.NumTxnsThatBatch
	for i := 0; i < BATCH_RANGE_LENGTH; i++ {
		metrics.NumPerSizeRange[i] += diff.NumPerSizeRange[i]
	}
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

	metrics.NestedP2WPKHOutputsSpent = stats.NestedP2WPKHOutputsSpent
	metrics.NestedP2WSHOutputsSpent = stats.NestedP2WSHOutputsSpent
	metrics.NativeP2WPKHOutputsSpent = stats.NestedP2WPKHOutputsSpent
	metrics.NativeP2WSHOutputsSpent = stats.NativeP2WSHOutputsSpent

	metrics.TxsSpendingNestedP2WPKHOutputs = stats.TxsSpendingNestedP2WPKHOutputs
	metrics.TxsSpendingNestedP2WSHOutputs = stats.TxsSpendingNestedP2WSHOutputs
	metrics.TxsSpendingNativeP2WPKHOutputs = stats.TxsSpendingNativeP2WPKHOutputs
	metrics.TxsSpendingNativeP2WSHOutputs = stats.TxsSpendingNativeP2WSHOutputs

	metrics.NewP2WPKHOutputs = stats.NewP2WPKHOutputs
	metrics.NewP2WSHOutputs = stats.NewP2WSHOutputs

	metrics.TxsCreatingP2WPKHOutputs = stats.TxsCreatingP2WPKHOutputs

	metrics.DustBins0 = stats.DustBins0
	metrics.DustBins1 = stats.DustBins1
	metrics.DustBins2 = stats.DustBins2
	metrics.DustBins3 = stats.DustBins3
	metrics.DustBins4 = stats.DustBins4
	metrics.DustBins5 = stats.DustBins5
	metrics.DustBins6 = stats.DustBins6
	metrics.DustBins7 = stats.DustBins7
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
	fields["utxo_size_increase"] = metrics.UTXOSizeIncrease

	fields["num_consolidating_txs"] = metrics.NumTxnsThatConsolidate
	fields["num_batching_txs"] = metrics.NumTxnsThatBatch

	// Avoid divide by 0 errors.
	if metrics.Txs != 0 {
		fields["percent_txs_signalling_RBF"] = float64(metrics.NumTxnsSignalingRBF) / float64(metrics.Txs)
		fields["percent_txs_batching"] = float64(metrics.NumTxnsThatBatch) / float64(metrics.Txs)
		fields["percent_txs_consolidating"] = float64(metrics.NumTxnsThatConsolidate) / float64(metrics.Txs)

		// Batch ranges =  [(1), (2), (3-4), (5-9), (10-49), (50-99), (100+)]
		fields["batch_range_0"] = float64(metrics.NumPerSizeRange[0]) / float64(metrics.Txs)
		fields["batch_range_1"] = float64(metrics.NumPerSizeRange[1]) / float64(metrics.Txs)
		fields["batch_range_2"] = float64(metrics.NumPerSizeRange[2]) / float64(metrics.Txs)
		fields["batch_range_3"] = float64(metrics.NumPerSizeRange[3]) / float64(metrics.Txs)
		fields["batch_range_4"] = float64(metrics.NumPerSizeRange[4]) / float64(metrics.Txs)
		fields["batch_range_5"] = float64(metrics.NumPerSizeRange[5]) / float64(metrics.Txs)
		fields["batch_range_6"] = float64(metrics.NumPerSizeRange[6]) / float64(metrics.Txs)

		fields["percent_txs_creating_native_segwit_outputs"] = float64(metrics.TxsCreatingP2WPKHOutputs+metrics.TxsCreatingP2WSHOutputs) / float64(metrics.Txs)
		fields["percent_txs_creating_P2WSH_outputs"] = float64(metrics.TxsCreatingP2WSHOutputs) / float64(metrics.Txs)
		fields["percent_txs_creating_P2WPKH_outputs"] = float64(metrics.TxsCreatingP2WPKHOutputs) / float64(metrics.Txs)

		fields["percent_txs_spending_native_segwit_outputs"] = float64(metrics.TxsSpendingNativeP2WPKHOutputs+metrics.TxsSpendingNativeP2WSHOutputs) / float64(metrics.Txs)
		fields["percent_txs_spending_native_P2WPKH_outputs"] = float64(metrics.TxsSpendingNativeP2WPKHOutputs) / float64(metrics.Txs)
		fields["percent_txs_spending_native_P2WSH_outputs"] = float64(metrics.TxsSpendingNativeP2WSHOutputs) / float64(metrics.Txs)
		fields["percent_txs_spending_nested_P2WPKH_outputs"] = float64(metrics.TxsSpendingNestedP2WPKHOutputs) / float64(metrics.Txs)
		fields["percent_txs_spending_nested_P2WSH_outputs"] = float64(metrics.TxsSpendingNestedP2WSHOutputs) / float64(metrics.Txs)

		fields["percent_txs_that_are_segwit_txs"] = float64(metrics.SegWitTxs) / float64(metrics.Txs)
	}

	fields["nested_P2WPKH_outputs_spent"] = metrics.NestedP2WPKHOutputsSpent
	fields["native_P2WPKH_outputs_spent"] = metrics.NativeP2WPKHOutputsSpent
	fields["nested_P2WSH_outputs_spent"] = metrics.NestedP2WSHOutputsSpent
	fields["native_P2WSH_outputs_spent"] = metrics.NativeP2WSHOutputsSpent

	// Avoid divide by 0 errors.
	if metrics.Ins != 0 {
		fields["percent_of_inputs_spending_nested_P2WPKH_output"] = float64(metrics.NestedP2WPKHOutputsSpent) / float64(metrics.Ins)
		fields["percent_of_inputs_spending_native_P2WPKH_outputs"] = float64(metrics.NativeP2WPKHOutputsSpent) / float64(metrics.Ins)
		fields["percent_of_inputs_spending_P2WPKH_outputs"] = float64(metrics.NativeP2WPKHOutputsSpent+metrics.NestedP2WPKHOutputsSpent) / float64(metrics.Ins)

		fields["percent_of_inputs_spending_nested_P2WSH_outputs"] = float64(metrics.NestedP2WSHOutputsSpent) / float64(metrics.Ins)
		fields["percent_of_inputs_spending_native_P2WSH_outputs"] = float64(metrics.NativeP2WSHOutputsSpent) / float64(metrics.Ins)
		fields["percent_of_inputs_spending_P2WSH_outputs"] = float64(metrics.NativeP2WSHOutputsSpent+metrics.NestedP2WSHOutputsSpent) / float64(metrics.Ins)

		fields["percent_of_inputs_spending_native_sw_outputs"] = float64(metrics.NativeP2WSHOutputsSpent+metrics.NativeP2WSHOutputsSpent) / float64(metrics.Ins)
	}

	/*
	   const CFeeRate dust_fee_rates[NUM_DUST_BINS] = {CFeeRate(1*1000), CFeeRate(5*1000), CFeeRate(10*1000), CFeeRate(25*1000),CFeeRate(50*1000), CFeeRate(100*1000), CFeeRate(250*1000), CFeeRate(500*1000), CFeeRate(1000*1000)};

	*/

	fields["dust_bin_0"] = float64(metrics.DustBins0)
	fields["dust_bin_1"] = float64(metrics.DustBins1)
	fields["dust_bin_2"] = float64(metrics.DustBins2)
	fields["dust_bin_3"] = float64(metrics.DustBins3)
	fields["dust_bin_4"] = float64(metrics.DustBins4)
	fields["dust_bin_5"] = float64(metrics.DustBins5)
	fields["dust_bin_6"] = float64(metrics.DustBins6)
	fields["dust_bin_7"] = float64(metrics.DustBins7)
	fields["dust_bin_8"] = float64(metrics.DustBins8)

	// Avoid divide by 0 errors.
	if metrics.Outs != 0 {
		fields["percent_new_outs_P2WPKH_outputs"] = float64(metrics.NewP2WPKHOutputs) / float64(metrics.Outs)
		fields["percent_new_outs_P2WSH_outputs"] = float64(metrics.NewP2WSHOutputs) / float64(metrics.Outs)

		fields["percent_outs_in_dust_bin_0"] = float64(metrics.DustBins0) / float64(metrics.Outs)
		fields["percent_outs_in_ddust_bin_1"] = float64(metrics.DustBins1) / float64(metrics.Outs)
		fields["percent_outs_in_dust_bin_2"] = float64(metrics.DustBins2) / float64(metrics.Outs)
		fields["percent_outs_in_dust_bin_3"] = float64(metrics.DustBins3) / float64(metrics.Outs)
		fields["percent_outs_in_dust_bin_4"] = float64(metrics.DustBins4) / float64(metrics.Outs)
		fields["percent_outs_in_dust_bin_5"] = float64(metrics.DustBins5) / float64(metrics.Outs)
		fields["percent_outs_in_dust_bin_6"] = float64(metrics.DustBins6) / float64(metrics.Outs)
		fields["percent_outs_in_dust_bin_7"] = float64(metrics.DustBins7) / float64(metrics.Outs)
		fields["percent_outs_in_dust_bin_8"] = float64(metrics.DustBins8) / float64(metrics.Outs)
	}

	// Avoid divide by 0 errors.
	if metrics.SegWitTxs != 0 {
		fields["percent_txs_native_segwit_over_total_sw_txs"] = float64(metrics.TxsSpendingNativeP2WSHOutputs+metrics.TxsSpendingNativeP2WPKHOutputs) / float64(metrics.SegWitTxs)
	}

	fields["num_txs_creating_P2WSH"] = metrics.TxsCreatingP2WSHOutputs
	fields["num_txs_creating_P2WPKH"] = metrics.TxsCreatingP2WPKHOutputs
	fields["num_txs_creating_native_segwit_outputs"] = metrics.TxsCreatingP2WPKHOutputs + metrics.TxsCreatingP2WSHOutputs
	fields["new_P2WPKH_outputs"] = metrics.NewP2WPKHOutputs
	fields["new_P2WSH_outputs"] = metrics.NewP2WSHOutputs
}
