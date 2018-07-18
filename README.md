# btc-dashboard
Fills an influx database with statistics about Bitcoin blocks retrieved by the (extended) getblockstats RPC.

## Stats Tracked
The program logs all statistics from getblockstats, and some statistics derived from these (e.g. percentage of transactions that have quality X).  

The exact statistics tracked are those set in `setInfluxFields`

## Requirements
Uses `expand-getblockstats` branch of https://github.com/bitcoinops/bitcoin with extended getblockstats RPC.
Uses `dashboard-rpc` branch of https://github.com/bitcoinops/btcd for RPC client that can use the extended getblockstats RPC.
Uses standard influxdb client.

Checkout the `dashboard-rpc` branch of btcd before running `go build`.

## Setup
### Set environment variables for influxdb
`DB` the name of the database in influxdb,  
`DB_USERNAME` influxdb username,  
`DB_PASSWORD` influxdb password,  
and optionally `DB_ADDR` if using a remote database. If not specified `DB_ADDR` defaults to "http://localhost:8086" which is the default address for a local influxdb instance.


### Set environment variables for bitcoind RPC access
`BITCOIND_USERNAME`, and
`BITCOIND_PASSWORD`
which should correspond to `rpcuser` and `rpcpassword` in the bitcoin.conf file.

optionally, set `BITCOIND_HOST` which defaults to "localhost:8332"

### Quick environment variable setup
Copy the file `example_env_file.txt` and edit to match your local configuration.
The command `export $(cat example_env_file.txt |xargs -L 1)` should set environment variables to match the file.

## Usage
Assumes previously mentioned environment variables are set, and correctly correspond to running instances of `influxdb` and `bitcoind`.

```
./btc-dashboard [OPTIONAL: -recovery] [OPTIONAL: -start=X] [OPTIONAL: -end=Y] [-workers=N]

```

Setting the `-workers=N` flag will cause the program to start `N` different RPC clients to do its work. The default value is 2.

The `-start=X` and `-end=Y` flags are used to specify the range of blockheights to analyze: [X, Y).


Not using the `-end=Y` flag will cause the program to do a live analysis. In this case, if a starting height is specified the live analysis will start at that height. Otherwise it starts 6 blocks behind the current blockheight of the chaintip. The analysis stays 6 blocks behind the tip in order to avoid implementing re-org logic.

The live analysis processes blocks one at a time with a single-worker as they come in.

The `-recovery` flag is described in the following section.

## Tracking Progress and Recovering from Failures
Because back-filling a database with the statistics from the entire Bitcoin blockchain can take a while, this program also implements some basic features to track progress of workers and features to recover from program failures.

Whenever a worker thread starts processing a range of blocks, it will write its progress to a file in a `worker-progress` directory. The file states the starting blockheight, the last blockheight analyzed, and the ending blockheight for this specific worker.

### Example
Suppose you ran the command `./btc-dashboard -start=1000 -end=2000 -workers=2`
and stopped the program before it completed. In the `worker-progress` directory you might see two files that have names similar to:  
`worker-0_07-18:11:10` and  
`worker-1_07-18:11:10`  

with contents that look something like:
```
Start=1000
Last=1234
End=1500
```

If you would like to restart the program continuing where these last workers left off, you can just run the command:  
`./btc-dashboard -recovery -workers=2`
which will start 2 workers to finish up this work, which will continue to mark their progress in the same files.

Workers that complete their assigned work delete their progress files.

## Results
Results from influxdb can be plugged into Grafana for visualization.



