# btc-dashboard
Implements a stats tracker for various stats derived from the Bitcoin `getblockstats` RPC. 
Stats are stored in a Postgres database and as JSON files.

Additionally, tracks mempool stats using `getmempoolinfo` and `getrawmempool` RPCs.

The dashboard is publicly available at: TODO: url
## Stats Tracked
The program logs all statistics from getblockstats, and some statistics derived from these (e.g. percentage of transactions that have quality X).

The exact statistics tracked are those set in the struct definition for `DashboardData` in `type_and_helpers.go`

An example JSON file with an explanation of the statistics stored is [STATS_TRACKED.md](STATS_TRACKED.md)

## Requirements
Uses `expand-getblockstats` branch of https://github.com/bitcoinops/bitcoin with extended getblockstats RPC.
Uses `dashboard-rpc` branch of https://github.com/bitcoinops/btcd for RPC client that can use the extended getblockstats RPC.
Uses `go-pg` as a Postgres client.

Checkout the `dashboard-rpc` branch of btcd before running `go build`.

## Setup
### Set environment variables for Postgres
`DB` the name of the database,
`DB_USERNAME` Postgres username,
`DB_PASSWORD` Postgres password,
and optionally `DB_ADDR` if using a remote database. If not specified `DB_ADDR` defaults to "http://localhost:5432" which is the default address for a local Postgres instance.

You don't have to specify a table, or create one yourself. This set of programs uses `go-pg`, which we use to create tables with schemas derived from Go struct definitions.

### Set environment variables for bitcoind RPC access
`BITCOIND_USERNAME`, and
`BITCOIND_PASSWORD`
which should correspond to `rpcuser` and `rpcpassword` in the bitcoin.conf file.

optionally, set `BITCOIND_HOST` which defaults to "localhost:8332"

### Quick environment variable setup
Copy the file `example_env_file.txt` and edit to match your local configuration.
The command `export $(cat example_env_file.txt |xargs -L 1)` should set environment variables to match the file.

## Usage
Assumes previously mentioned environment variables are set, and correctly correspond to running instances of `postgres` and `bitcoind`.

```
./btc-dashboard [OPTIONAL: -recovery] [OPTIONAL: -start=X] [OPTIONAL: -end=Y] [-workers=N]

```

### Modes of Operation
* `-mempool` Setting this flag starts a mempool tracker that continuously stores data derived from RPCs into a database. It does not halt by itself, but is safe to stop (catches SIGINT and SIGTERM after all writes are finished).

* `-recovery`Starts workers on any progress files left over from previously unfinished runs.

* `-insert-json` Uploads contents of every JSON file in the default directory and uploads them into Postgres.

* `-json=[true,false]`  If set, every `DashboardData` struct inserted into the database will also be saved as a JSON file. Defaults to `true`. The default directory is `./db-backup`.

* `-email` Setting this flag enables the program to send emails in case of failure (i.e. places where `log.Fatal` is called). Requires `EMAIL_ADDR` and `EMAIL_PASSWORD` to be set for sending email account, and `RECIPIENT_EMAILS` (comma-separated list of email addresses) for all recipients.

Setting the `-workers=N` flag will cause the program to start `N` different RPC clients to do its work. The default value is 2.

The `-start=X` and `-end=Y` flags are used to specify the range of blockheights to analyze: [X, Y).


Not using the `-end=Y` flag will cause the program to do a live analysis. In this case, if a starting height is specified the live analysis will start at that height. Otherwise it starts 6 blocks behind the current blockheight of the chaintip. The analysis stays 6 blocks behind the tip in order to avoid implementing re-org logic.

The live analysis processes blocks one at a time with a single-worker as they come in.

Otherwise with at least the `-end` flag set, the program starts a backfill analysis from the interval [start, end), where start defaults to 0.


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
Results from the database can be plugged into Grafana for visualization.
