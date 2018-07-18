package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

func main() {
	cleanUpFromFile(os.Args[1])
}

/*
 cleanUpFromFile opens the given file and parses the heights given.
 It then makes a directory with worker progress files that reflect gaps in
 heights of the given file.

 You can create a file that fits the expected format as follows:
 influx -database 'dashboard_db_v2' -precision 'rfc3339' -execute 'show tag values with key="height"' > heights.txt

*/
func cleanUpFromFile(fileName string) {
	contentsBytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatal("Error reading file: ", err)
	}
	contents := string(contentsBytes)
	lines := strings.Split(contents, "\n")

	lines = lines[2:]
	heights := make([]int, len(lines)-1)
	for i, line := range lines {
		words := strings.Fields(line)
		if len(words) != 2 {
			continue
		}

		height, err := strconv.Atoi(words[1])
		if err != nil {
			log.Fatal(err)
		}

		heights[i] = height
	}

	// Sort heights so we can find gaps easily.
	sort.Ints(heights)

	currentDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	// Create the progress directory.
	formattedTime := time.Now().Format("01-02:15:04")
	workerProgressDir := currentDir + "/cleanup-worker-progress-" + formattedTime
	err = os.Mkdir(workerProgressDir, 0777)
	if err != nil {
		log.Fatal(err)
	}

	pCount := 0

	for i := 0; i < len(heights)-1; i++ {
		if (heights[i] + 1) != heights[i+1] {
			progress := fmt.Sprintf("Start=%v\nLast=%v\nEnd=%v", heights[i], heights[i], heights[i+1])

			// Create file to record progress in.
			workFile := fmt.Sprintf("%v/gap-worker-%v_%v", workerProgressDir, pCount, formattedTime)
			file, err := os.Create(workFile)
			defer file.Close()
			if err != nil {
				log.Fatal(err)
			}

			// Record progress in file.
			_, err = file.WriteAt([]byte(progress), 0)
			if err != nil {
				log.Fatal(err)
			}

			pCount++
		}
	}
}
