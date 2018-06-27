# btc-dashboard
## Requirements
Uses bitcoind fork https://github.com/marcinja/bitcoin/tree/expand-getblockstats with extended getblockstats RPC that gives more stats.

Uses btcd fork https://github.com/marcinja/btcd/tree/dashboard-rpc for rpcclient with handler for (extended) getblockstats.

## Setup
Set environment variables for influxdb: DB, DB\_USERNAME, DB\_PASSWORD. Then setup environment variables for bitcoind RPC access: BITCOIND\_HOST, BITCOIND\_USERNAME, BITCOIND\_PASSWORD. To do this you can edit example\_env\_file.txt and run the command `export (cat env_file.txt |xargs -L 1)`

Start `influxd` and create a database with name $DB.

Then run `go build`

## Usage
```
./btc-dashboard [OPTIONAL: start_blockheight] [OPTIONAL: end_blockheight]
```
Running the binary with one integer parameter will print out the result of the getblockstats RPC at the given blockheight.

Running the above command with 2 integer parameters will start the analysis process that enters statistics about every block in the given range into influxdb. Several workers are created in this process, and their progress is tracked in files under a directory `worker-progress`.

Running the binary without any arguments will start a recovery process that reads `worker-progress` files left over by failures and finishes any work that is unfinished. It then starts a live analysis of incoming blocks.

Results from influxdb can be plugged into Grafana for visualization.

## Stats Tracked
(TBD) Whatever fields are set in `setInfluxFields`
