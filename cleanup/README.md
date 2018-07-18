## cleanup
Creates a `worker-progress` directory to fill in gaps found in a list of blockheights processed by influx.

### Usage
  You can create a file that fits the expected format as follows:
 `influx -database 'database_name' -precision 'rfc3339' -execute 'show tag values with key="height"' > heights.txt`
 
 Running the command: `./cleanup heights.txt` creates a directory with progress files for each gap in the heights.
