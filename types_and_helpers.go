package main

import (
	"github.com/btcsuite/btcd/btcjson"
)

// The Data type encapsulates all future tables added to the dashboard.
// Each field should correspond to a different table and have its own struct type.
// This makes using go-pg easy because of ORM.
type Data struct {
	Version          int64           `json:"version"`
	DashboardDataRow DashboardDataV2 `json:"dashboard_data"`

	// Future tables below:
}

// dataBatch is used internally for collecting data for batch insertions.
type dataBatch struct {
	versions          []int64
	dashboardDataRows []DashboardDataV2
}

type BlockStats struct {
	*btcjson.GetBlockStatsResult
}

func (metrics BlockStats) transformToDashboardData() DashboardDataV2 {
	data := DashboardDataV2{}
	data.Id = metrics.Height

	data.Avg_fee = metrics.AverageFee
	data.Avg_fee_rate = metrics.AverageFeeRate
	data.Avg_tx_size = metrics.AverageTxSize

	data.Hash = metrics.Hash
	data.Height = metrics.Height
	data.Num_inputs = metrics.Ins

	data.Max_fee = metrics.MaxFee
	data.Max_fee_rate = metrics.MaxFeeRate
	data.Max_tx_size = metrics.MaxTxSize

	data.Median_fee = metrics.MedianFee
	data.Median_time = metrics.MedianTime
	data.Median_tx_size = metrics.MedianTxSize

	data.Min_fee = metrics.MinFee
	data.Min_fee_rate = metrics.MinFeeRate
	data.Min_tx_size = metrics.MinTxSize

	data.Feerate_percentiles = metrics.FeeratePercentiles

	data.Num_outputs = metrics.Outs
	data.Subsidy = metrics.Subsidy
	data.Segwit_total_size = metrics.SegWitTotalSize
	data.Segwit_total_weight = metrics.SegWitTotalWeight
	data.Num_segwit_txs = metrics.SegWitTxs

	data.Time = metrics.Time
	data.Total_amount_out = metrics.TotalOut
	data.Total_block_size = metrics.TotalSize
	data.Total_weight = metrics.TotalWeight
	data.Total_fee = metrics.TotalFee

	data.Num_txs = metrics.Txs
	data.Utxo_increase = metrics.UTXOIncrease
	data.Utxo_size_increase = metrics.UTXOSizeIncrease

	data.Nested_P2WPKH_outputs_spent = metrics.NestedP2WPKHOutputsSpent
	data.Native_P2WPKH_outputs_spent = metrics.NativeP2WPKHOutputsSpent
	data.Nested_P2WSH_outputs_spent = metrics.NestedP2WSHOutputsSpent
	data.Native_P2WSH_outputs_spent = metrics.NativeP2WSHOutputsSpent

	data.Txs_spending_nested_p2wpkh_outputs = metrics.TxsSpendingNestedP2WPKHOutputs
	data.Txs_spending_nested_p2wsh_outputs = metrics.TxsSpendingNestedP2WSHOutputs
	data.Txs_spending_native_p2wpkh_outputs = metrics.TxsSpendingNativeP2WPKHOutputs
	data.Txs_spending_native_p2wsh_outputs = metrics.TxsSpendingNativeP2WSHOutputs

	data.Value_of_native_P2WPKH_outputs_spent = metrics.Value_of_native_P2WPKH_outputs_spent
	data.Value_of_native_P2WSH_outputs_spent = metrics.Value_of_native_P2WSH_outputs_spent
	data.Value_of_nested_P2WPKH_outputs_spent = metrics.Value_of_nested_P2WPKH_outputs_spent
	data.Value_of_nested_P2WSH_outputs_spent = metrics.Value_of_nested_P2WSH_outputs_spent

	data.Value_of_native_P2WPKH_outputs_created = metrics.Value_of_native_P2WPKH_outputs_created
	data.Value_of_native_P2WSH_outputs_created = metrics.Value_of_native_P2WSH_outputs_created

	data.New_P2WPKH_outputs = metrics.NewP2WPKHOutputs
	data.New_P2WSH_outputs = metrics.NewP2WSHOutputs

	data.Txs_creating_P2WPKH = metrics.TxsCreatingP2WPKHOutputs
	data.Txs_creating_P2WSH = metrics.TxsCreatingP2WSHOutputs

	data.Txs_signalling_opt_in_rbf = metrics.TxsSignallingOptInRBF
	data.Consolidating_txs = metrics.TxsConsolidating
	data.Outputs_consolidated = metrics.OutputsConsolidated
	data.Batching_txs = metrics.TxsBatching

	data.Txs_by_output_count = metrics.OutputCountBins
	data.Dust_output_count = metrics.DustBins

	data.Mto_consolidations = metrics.Mto_consolidations
	data.Mto_output_count = metrics.Mto_output_count
	data.Mto_total_value = metrics.Mto_total_value

	// Derived added below /////////////////////////////////////////////////////
	data.Percent_txs_by_output_count = make([]float64, len(data.Txs_by_output_count))
	data.Dust_output_percentages = make([]float64, len(data.Dust_output_count))
	if data.Num_outputs != 0 {
		for i := 0; i < len(data.Txs_by_output_count); i++ {
			data.Percent_txs_by_output_count[i] = float64(data.Txs_by_output_count[i]) / float64(data.Num_outputs)
		}
		for i := 0; i < len(data.Dust_output_percentages); i++ {
			data.Dust_output_percentages[i] = float64(data.Dust_output_count[i]) / float64(data.Num_outputs)
		}
	}

	data.Txs_spending_native_sw_outputs = metrics.TxsSpendingNativeP2WPKHOutputs + metrics.TxsSpendingNativeP2WSHOutputs
	data.Txs_spending_nested_sw_outputs = metrics.TxsSpendingNestedP2WPKHOutputs + metrics.TxsSpendingNestedP2WSHOutputs

	data.Num_txs_creating_native_segwit_outputs = metrics.TxsCreatingP2WPKHOutputs + metrics.TxsCreatingP2WSHOutputs
	if metrics.Txs != 0 {
		data.Percent_txs_signalling_opt_in_RBF = float64(metrics.TxsSignallingOptInRBF) / float64(metrics.Txs)
		data.Percent_txs_batching = float64(metrics.TxsBatching) / float64(metrics.Txs)
		data.Percent_txs_consolidating = float64(metrics.TxsConsolidating) / float64(metrics.Txs)
		data.Percent_txs_creating_native_segwit_outputs = float64(metrics.TxsCreatingP2WPKHOutputs+metrics.TxsCreatingP2WSHOutputs) / float64(metrics.Txs)
		data.Percent_txs_creating_P2WSH_outputs = float64(metrics.TxsCreatingP2WSHOutputs) / float64(metrics.Txs)
		data.Percent_txs_creating_P2WPKH_outputs = float64(metrics.TxsCreatingP2WPKHOutputs) / float64(metrics.Txs)

		data.Percent_txs_spending_native_segwit_outputs = float64(metrics.TxsSpendingNativeP2WPKHOutputs+metrics.TxsSpendingNativeP2WSHOutputs) / float64(metrics.Txs)
		data.Percent_txs_spending_nested_segwit_outputs = float64(metrics.TxsSpendingNestedP2WPKHOutputs+metrics.TxsSpendingNestedP2WSHOutputs) / float64(metrics.Txs)
		data.Percent_txs_spending_native_P2WPKH_outputs = float64(metrics.TxsSpendingNativeP2WPKHOutputs) / float64(metrics.Txs)
		data.Percent_txs_spending_native_P2WSH_outputs = float64(metrics.TxsSpendingNativeP2WSHOutputs) / float64(metrics.Txs)
		data.Percent_txs_spending_nested_P2WPKH_outputs = float64(metrics.TxsSpendingNestedP2WPKHOutputs) / float64(metrics.Txs)
		data.Percent_txs_spending_nested_P2WSH_outputs = float64(metrics.TxsSpendingNestedP2WSHOutputs) / float64(metrics.Txs)
		data.Percent_txs_spending_P2WSH_outputs = float64(metrics.TxsSpendingNativeP2WSHOutputs+metrics.TxsSpendingNestedP2WSHOutputs) / float64(metrics.Txs)
		data.Percent_txs_spending_P2WPKH_outputs = float64(metrics.TxsSpendingNativeP2WPKHOutputs+metrics.TxsSpendingNestedP2WPKHOutputs) / float64(metrics.Txs)
		data.Percent_txs_that_are_segwit_txs = float64(metrics.SegWitTxs) / float64(metrics.Txs)

	}

	if metrics.Ins != 0 {
		data.Percent_of_inputs_spending_nested_P2WPKH_output = float64(metrics.NestedP2WPKHOutputsSpent) / float64(metrics.Ins)
		data.Percent_of_inputs_spending_native_P2WPKH_outputs = float64(metrics.NativeP2WPKHOutputsSpent) / float64(metrics.Ins)
		data.Percent_of_inputs_spending_P2WPKH_outputs = float64(metrics.NativeP2WPKHOutputsSpent+metrics.NestedP2WPKHOutputsSpent) / float64(metrics.Ins)
		data.Percent_of_inputs_spending_nested_P2WSH_outputs = float64(metrics.NestedP2WSHOutputsSpent) / float64(metrics.Ins)
		data.Percent_of_inputs_spending_native_P2WSH_outputs = float64(metrics.NativeP2WSHOutputsSpent) / float64(metrics.Ins)
		data.Percent_of_inputs_spending_P2WSH_outputs = float64(metrics.NativeP2WSHOutputsSpent+metrics.NestedP2WSHOutputsSpent) / float64(metrics.Ins)
		data.Percent_of_inputs_spending_native_sw_outputs = float64(metrics.NativeP2WSHOutputsSpent+metrics.NativeP2WSHOutputsSpent) / float64(metrics.Ins)
		data.Percent_inputs_consolidated = float64(metrics.OutputsConsolidated) / float64(metrics.Ins)
	}

	if metrics.SegWitTxs != 0 {
		data.Percent_sw_txs_that_are_native_sw = float64(metrics.TxsSpendingNativeP2WSHOutputs+metrics.TxsSpendingNativeP2WPKHOutputs) / float64(metrics.SegWitTxs)
	}

	return data
}

// Custom struct type for custom struct tags and to add derived fields (e.g. all the 'percentage' fields)
type DashboardDataV2 struct {
	Id int64 `json:"id,omit_empty" sql:",notnull"`

	Avg_fee      int64 `json:"avg_fee" sql:",notnull"`
	Avg_fee_rate int64 `json:"avg_fee_rate" sql:",notnull"`
	Avg_tx_size  int64 `json:"avg_tx_size" sql:",notnull"`

	Hash       string `json:"hash" sql:",notnull"`
	Height     int64  `json:"height" sql:",notnull"`
	Num_inputs int64  `json:"num_inputs" sql:",notnull"`

	Max_fee      int64 `json:"max_fee" sql:",notnull"`
	Max_fee_rate int64 `json:"max_fee_rate" sql:",notnull"`
	Max_tx_size  int64 `json:"max_tx_size" sql:",notnull"`

	Median_fee     int64 `json:"median_fee" sql:",notnull"`
	Median_time    int64 `json:"median_time"`
	Median_tx_size int64 `json:"median_tx_size" sql:",notnull"`

	Min_fee      int64 `json:"min_fee" sql:",notnull"`
	Min_fee_rate int64 `json:"min_fee_rate" sql:",notnull"`
	Min_tx_size  int64 `json:"min_tx_size" sql:",notnull"`

	Feerate_percentiles []int `json:"feerate_percentiles" pg:",array" sql:",notnull"`

	Num_outputs         int64 `json:"num_outputs" sql:",notnull"`
	Subsidy             int64 `json:"subsidy" sql:",notnull"`
	Segwit_total_size   int64 `json:"segwit_total_size" sql:",notnull"`
	Segwit_total_weight int64 `json:"segwit_total_weight" sql:",notnull"`
	Num_segwit_txs      int64 `json:"num_segwit_txs" sql:",notnull"`

	Time             int64 `json:"time" sql:",notnull"`
	Total_amount_out int64 `json:"total_amount_out" sql:",notnull"`
	Total_block_size int64 `json:"total_block_size" sql:",notnull"`
	Total_weight     int64 `json:"total_weight" sql:",notnull"`
	Total_fee        int64 `json:"total_fee" sql:",notnull"`

	Num_txs            int64 `json:"num_txs" sql:",notnull"`
	Utxo_increase      int64 `json:"utxo_increase" sql:",notnull"`
	Utxo_size_increase int64 `json:"utxo_size_increase" sql:",notnull"`

	Nested_P2WPKH_outputs_spent int64 `json:"nested_P2WPKH_outputs_spent" sql:",notnull"`
	Nested_P2WSH_outputs_spent  int64 `json:"nested_P2WSH_outputs_spent" sql:",notnull"`
	Native_P2WPKH_outputs_spent int64 `json:"native_P2WPKH_outputs_spent" sql:",notnull"`
	Native_P2WSH_outputs_spent  int64 `json:"native_P2WSH_outputs_spent" sql:",notnull"`

	Txs_spending_nested_p2wpkh_outputs int64 `json:"txs_spending_nested_p2wpkh_outputs" sql:",notnull"`
	Txs_spending_nested_p2wsh_outputs  int64 `json:"txs_spending_nested_p2wsh_outputs" sql:",notnull"`
	Txs_spending_native_p2wpkh_outputs int64 `json:"txs_spending_native_p2wpkh_outputs" sql:",notnull"`
	Txs_spending_native_p2wsh_outputs  int64 `json:"txs_spending_native_p2wsh_outputs" sql:",notnull"`

	Value_of_nested_P2WPKH_outputs_spent   int64 `json:"value_of_nested_P2WPKH_outputs_spent" sql:",notnull"`
	Value_of_nested_P2WSH_outputs_spent    int64 `json:"value_of_nested_P2WSH_outputs_spent" sql:",notnull"`
	Value_of_native_P2WPKH_outputs_spent   int64 `json:"value_of_native_P2WPKH_outputs_spent" sql:",notnull"`
	Value_of_native_P2WSH_outputs_spent    int64 `json:"value_of_native_P2WSH_outputs_spent" sql:",notnull"`
	Value_of_native_P2WPKH_outputs_created int64 `json:"value_of_native_P2WPKH_outputs_created" sql:",notnull"`
	Value_of_native_P2WSH_outputs_created  int64 `json:"value_of_native_P2WSH_outputs_created" sql:",notnull"`

	New_P2WPKH_outputs int64 `json:"new_P2WPKH_outputs" sql:",notnull"`
	New_P2WSH_outputs  int64 `json:"new_P2WSH_outputs" sql:",notnull"`

	Txs_creating_P2WPKH int64 `json:"txs_creating_P2WPKH_outputs" sql:",notnull"`
	Txs_creating_P2WSH  int64 `json:"txs_creating_P2WSH_outputs" sql:",notnull"`

	Txs_signalling_opt_in_rbf int64 `json:"txs_signalling_opt_in_rbf" sql:",notnull"`
	Consolidating_txs         int64 `json:"consolidating_txs" sql:",notnull"`
	Outputs_consolidated      int64 `json:"outputs_consolidated" sql:",notnull"`
	Batching_txs              int64 `json:"batching_txs" sql:",notnull"`

	Txs_by_output_count []int64 `json:"txs_by_output_count" pg:",array" sql:",notnull"`
	Dust_output_count   []int64 `json:"dust_output_count" pg:",array" sql:",notnull"`

	Mto_consolidations int64 `json:"mto_consolidations" sql:",notnull"`
	Mto_output_count   int64 `json:"mto_output_count" sql:",notnull"`
	Mto_total_value    int64 `json:"mto_total_value" sql:",notnull"`

	// Fields derived from getblockstats fields below.
	Num_txs_creating_native_segwit_outputs int64 `json:"num_txs_creating_native_segwit_outputs" sql:",notnull"`

	Txs_spending_native_sw_outputs int64 `json:"txs_spending_native_sw_outputs" sql:",notnull"`
	Txs_spending_nested_sw_outputs int64 `json:"txs_spending_nested_sw_outputs" sql:",notnull"`

	Percent_inputs_consolidated     float64 `json:"percent_inputs_consolidated" sql:",notnull"`
	Percent_new_outs_P2WPKH_outputs float64 `json:"percent_new_outs_P2WPKH_outputs" sql:",notnull"`
	Percent_new_outs_P2WSH_outputs  float64 `json:"percent_new_outs_P2WSH_outputs" sql:",notnull"`

	Percent_txs_by_output_count []float64 `json:"percent_txs_by_output_count" pg:",array" sql:",notnull"`
	Dust_output_percentages     []float64 `json:"dust_output_percentages" pg:",array" sql:",notnull"`

	Percent_of_inputs_spending_P2WPKH_outputs        float64 `json:"percent_of_inputs_spending_P2WPKH_outputs" sql:",notnull"`
	Percent_of_inputs_spending_P2WSH_outputs         float64 `json:"percent_of_inputs_spending_P2WSH_outputs" sql:",notnull"`
	Percent_of_inputs_spending_native_P2WPKH_outputs float64 `json:"percent_of_inputs_spending_native_P2WPKH_outputs" sql:",notnull"`
	Percent_of_inputs_spending_native_P2WSH_outputs  float64 `json:"percent_of_inputs_spending_native_P2WSH_outputs" sql:",notnull"`
	Percent_of_inputs_spending_native_sw_outputs     float64 `json:"percent_of_inputs_spending_native_sw_outputs" sql:",notnull"`
	Percent_of_inputs_spending_nested_sw_outputs     float64 `json:"percent_of_inputs_spending_nested_sw_outputs" sql:",notnull"`
	Percent_of_inputs_spending_nested_P2WPKH_output  float64 `json:"percent_of_inputs_spending_nested_P2WPKH_output" sql:",notnull"`
	Percent_of_inputs_spending_nested_P2WSH_outputs  float64 `json:"percent_of_inputs_spending_nested_P2WSH_outputs" sql:",notnull"`

	Percent_txs_spending_P2WPKH_outputs        float64 `json:"percent_txs_spending_P2WPKH_outputs" sql:",notnull"`
	Percent_txs_spending_P2WSH_outputs         float64 `json:"percent_txs_spending_P2WSH_outputs" sql:",notnull"`
	Percent_txs_spending_native_P2WPKH_outputs float64 `json:"percent_txs_spending_native_P2WPKH_outputs" sql:",notnull"`
	Percent_txs_spending_native_P2WSH_outputs  float64 `json:"percent_txs_spending_native_P2WSH_outputs" sql:",notnull"`
	Percent_txs_spending_native_segwit_outputs float64 `json:"percent_txs_spending_native_segwit_outputs" sql:",notnull"`
	Percent_txs_spending_nested_segwit_outputs float64 `json:"percent_txs_spending_nested_segwit_outputs" sql:",notnull"`
	Percent_txs_spending_nested_P2WPKH_outputs float64 `json:"percent_txs_spending_nested_P2WPKH_outputs" sql:",notnull"`
	Percent_txs_spending_nested_P2WSH_outputs  float64 `json:"percent_txs_spending_nested_P2WSH_outputs" sql:",notnull"`

	Percent_txs_creating_P2WPKH_outputs        float64 `json:"percent_txs_creating_P2WPKH_outputs" sql:",notnull"`
	Percent_txs_creating_P2WSH_outputs         float64 `json:"percent_txs_creating_P2WSH_outputs" sql:",notnull"`
	Percent_txs_creating_native_segwit_outputs float64 `json:"percent_txs_creating_native_segwit_outputs" sql:",notnull"`

	Percent_sw_txs_that_are_native_sw float64 `json:"percent_sw_txs_that_are_native_sw" sql:",notnull"`
	Percent_txs_that_are_segwit_txs   float64 `json:"percent_txs_that_are_segwit_txs" sql:",notnull"`

	Percent_txs_signalling_opt_in_RBF float64 `json:"percent_txs_signalling_opt_in_RBF" sql:",notnull"`
	Percent_txs_consolidating         float64 `json:"percent_txs_consolidating" sql:",notnull"`
	Percent_txs_batching              float64 `json:"percent_txs_batching" sql:",notnull"`
}
