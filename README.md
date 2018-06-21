# btc-dashboard
## Requirements
Uses bitcoind fork https://github.com/marcinja/bitcoin/tree/expand-getblockstats with extended getblockstats RPC that gives more stats.

Uses btcd fork https://github.com/marcinja/btcd/tree/dashboard-rpc for rpcclient with handler for (extended) getblockstats.

## Setup
Set environment variables for influxdb: DB, DB_USERNAME, DB_PASSWORD. Then setup environment variables for bitcoind RPC access: BITCOIND_HOST, BITCOIND_USERNAME, BITCOIND_PASSWORD.

Start `influxd` and create a database with name DB.

Then run `go build`

## Usage
```
./btc-dashboard [start_blockheight] [end_blockheight]
```
Running the above command will start the analysis process that enters statistics about every block in the given range into influxdb.

Results can be plugged into Grafana for visualization.

## Stats Tracked
(TBD) Whatever fields are set in `setInfluxFields`
