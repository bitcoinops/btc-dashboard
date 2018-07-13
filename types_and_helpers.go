package dashboard

import (
	"fmt"
	"github.com/btcsuite/btcd/btcjson"
	"strconv"
)

const PROFILE = false
const BLOCK_NUM_DIFF = 6

// Batch ranges =  [(1), (2), (3-4), (5-9), (10-49), (50-99), (100+)]
const BATCH_RANGE_LENGTH = 7

const MAX_ATTEMPTS = 3 // max number of DB write attempts before giving up

type BlockStats struct {
	*btcjson.GetBlockStatsResult
}

func (metrics BlockStats) setInfluxTags(tags map[string]string, height int64) {
	tags["height"] = strconv.Itoa(int(height))
}

func (metrics BlockStats) setInfluxFields(fields map[string]interface{}) {
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

	fields["nested_P2WPKH_outputs_spent"] = metrics.NestedP2WPKHOutputsSpent
	fields["native_P2WPKH_outputs_spent"] = metrics.NativeP2WPKHOutputsSpent
	fields["nested_P2WSH_outputs_spent"] = metrics.NestedP2WSHOutputsSpent
	fields["native_P2WSH_outputs_spent"] = metrics.NativeP2WSHOutputsSpent

	fields["txs_spending_nested_p2wpkh_outputs"] = metrics.TxsSpendingNestedP2WPKHOutputs
	fields["txs_spending_nested_p2wsh_outputs"] = metrics.TxsSpendingNestedP2WSHOutputs
	fields["txs_spending_native_p2wpkh_outputs"] = metrics.TxsSpendingNativeP2WPKHOutputs
	fields["txs_spending_native_p2wsh_outputs"] = metrics.TxsSpendingNativeP2WSHOutputs

	fields["new_P2WPKH_outputs"] = metrics.NewP2WPKHOutputs
	fields["new_P2WSH_outputs"] = metrics.NewP2WSHOutputs
	fields["num_txs_creating_P2WPKH"] = metrics.TxsCreatingP2WPKHOutputs
	fields["num_txs_creating_P2WSH"] = metrics.TxsCreatingP2WSHOutputs

	fields["num_txs_signalling_rbf"] = metrics.TxsSignallingRBF
	fields["num_consolidating_txs"] = metrics.TxsConsolidating
	fields["num_outputs_consolidated"] = metrics.OutputsConsolidated
	fields["num_batching_txs"] = metrics.TxsBatching

	nDustBins := len(metrics.DustBins)
	for i := 0; i < nDustBins; i++ {
		fieldName := fmt.Sprintf("dust_bin_%v", i)
		fields[fieldName] = metrics.DustBins[i]
	}

	nOutputCountBins := len(metrics.OutputCountBins)
	for i := 0; i < nOutputCountBins; i++ {
		fieldName := fmt.Sprintf("output_count_bin_%v", i)
		fields[fieldName] = metrics.OutputCountBins[i]
	}

	// Derived fields added below /////////////////////////////////////////////////////

	fields["num_txs_creating_native_segwit_outputs"] = metrics.TxsCreatingP2WPKHOutputs + metrics.TxsCreatingP2WSHOutputs
	// Avoid divide by 0 errors.
	if metrics.Txs != 0 {
		// Batch ranges =  [(1), (2), (3-4), (5-9), (10-49), (50-99), (100+)]
		for i := 0; i < nOutputCountBins; i++ {
			fieldName := fmt.Sprintf("batch_range_%v", i)
			fields[fieldName] = float64(metrics.OutputCountBins[i]) / float64(metrics.Txs)
		}

		fields["percent_txs_signalling_RBF"] = float64(metrics.TxsSignallingRBF) / float64(metrics.Txs)
		fields["percent_txs_batching"] = float64(metrics.TxsBatching) / float64(metrics.Txs)
		fields["percent_txs_consolidating"] = float64(metrics.TxsConsolidating) / float64(metrics.Txs)
		fields["percent_txs_creating_native_segwit_outputs"] = float64(metrics.TxsCreatingP2WPKHOutputs+metrics.TxsCreatingP2WSHOutputs) / float64(metrics.Txs)
		fields["percent_txs_creating_P2WSH_outputs"] = float64(metrics.TxsCreatingP2WSHOutputs) / float64(metrics.Txs)
		fields["percent_txs_creating_P2WPKH_outputs"] = float64(metrics.TxsCreatingP2WPKHOutputs) / float64(metrics.Txs)
		fields["percent_txs_spending_native_segwit_outputs"] = float64(metrics.TxsSpendingNativeP2WPKHOutputs+metrics.TxsSpendingNativeP2WSHOutputs) / float64(metrics.Txs)
		fields["percent_txs_spending_native_P2WPKH_outputs"] = float64(metrics.TxsSpendingNativeP2WPKHOutputs) / float64(metrics.Txs)
		fields["percent_txs_spending_native_P2WSH_outputs"] = float64(metrics.TxsSpendingNativeP2WSHOutputs) / float64(metrics.Txs)
		fields["percent_txs_spending_nested_P2WPKH_outputs"] = float64(metrics.TxsSpendingNestedP2WPKHOutputs) / float64(metrics.Txs)
		fields["percent_txs_spending_nested_P2WSH_outputs"] = float64(metrics.TxsSpendingNestedP2WSHOutputs) / float64(metrics.Txs)
		fields["percent_txs_spending_P2WSH_outputs"] = float64(metrics.TxsSpendingNativeP2WSHOutputs+metrics.TxsSpendingNestedP2WSHOutputs) / float64(metrics.Txs)
		fields["percent_txs_spending_P2WPKH_outputs"] = float64(metrics.TxsSpendingNativeP2WPKHOutputs+metrics.TxsSpendingNestedP2WPKHOutputs) / float64(metrics.Txs)
		fields["percent_txs_that_are_segwit_txs"] = float64(metrics.SegWitTxs) / float64(metrics.Txs)
	}

	// Avoid divide by 0 errors.
	if metrics.Ins != 0 {
		fields["percent_of_inputs_spending_nested_P2WPKH_output"] = float64(metrics.NestedP2WPKHOutputsSpent) / float64(metrics.Ins)
		fields["percent_of_inputs_spending_native_P2WPKH_outputs"] = float64(metrics.NativeP2WPKHOutputsSpent) / float64(metrics.Ins)
		fields["percent_of_inputs_spending_P2WPKH_outputs"] = float64(metrics.NativeP2WPKHOutputsSpent+metrics.NestedP2WPKHOutputsSpent) / float64(metrics.Ins)
		fields["percent_of_inputs_spending_nested_P2WSH_outputs"] = float64(metrics.NestedP2WSHOutputsSpent) / float64(metrics.Ins)
		fields["percent_of_inputs_spending_native_P2WSH_outputs"] = float64(metrics.NativeP2WSHOutputsSpent) / float64(metrics.Ins)
		fields["percent_of_inputs_spending_P2WSH_outputs"] = float64(metrics.NativeP2WSHOutputsSpent+metrics.NestedP2WSHOutputsSpent) / float64(metrics.Ins)
		fields["percent_of_inputs_spending_native_sw_outputs"] = float64(metrics.NativeP2WSHOutputsSpent+metrics.NativeP2WSHOutputsSpent) / float64(metrics.Ins)
		fields["percent_inputs_consolidated"] = float64(metrics.OutputsConsolidated) / float64(metrics.Ins)
	}

	// Avoid divide by 0 errors.
	if metrics.Outs != 0 {
		fields["percent_new_outs_P2WPKH_outputs"] = float64(metrics.NewP2WPKHOutputs) / float64(metrics.Outs)
		fields["percent_new_outs_P2WSH_outputs"] = float64(metrics.NewP2WSHOutputs) / float64(metrics.Outs)

		for i := 0; i < nDustBins; i++ {
			fieldName := fmt.Sprintf("percent_new_outs_in_dust_bin_%v", i)
			fields[fieldName] = float64(metrics.DustBins[i]) / float64(metrics.Outs)
		}
	}

	// Avoid divide by 0 errors.
	if metrics.SegWitTxs != 0 {
		fields["percent_txs_native_segwit_over_total_sw_txs"] = float64(metrics.TxsSpendingNativeP2WSHOutputs+metrics.TxsSpendingNativeP2WPKHOutputs) / float64(metrics.SegWitTxs)
	}
}
