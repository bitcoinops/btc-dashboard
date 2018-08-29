package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	gomail "gopkg.in/mail.v2"
)

func storeDataAsFile(data Data) {
	dataFileName := fmt.Sprintf("%v/%v.json", JSON_DIR, data.DashboardDataRow.Height)
	dataFile, err := os.Create(dataFileName)
	if err != nil {
		fmt.Println("Error creating file", err)
	}

	enc := json.NewEncoder(dataFile)
	enc.Encode(data)

	dataFile.Close()
}

// parseProgress takes in the contents of a worker-progress file
// and returns the starting height, the last height completed, and the end height.
func parseProgress(contents string) []int {
	lines := strings.Split(contents, "\n")
	result := make([]int, 0)

	for _, line := range lines {
		split := strings.Split(line, "=")

		if len(split) < 2 {
			continue
		}
		height, err := strconv.Atoi(split[1])
		if err != nil {
			fatal("Error in parseProgress, strconv parsing: ", err)
		}

		result = append(result, height)
	}

	return result
}

// logProgressToFile records the progress of a worker to a given file.
func logProgressToFile(start, last, end int, file *os.File) {
	// Record progress in file.
	progress := fmt.Sprintf("Start=%v\nLast=%v\nEnd=%v", start, last, end)
	_, err := file.WriteAt([]byte(progress), 0)

	if err != nil {
		fatal("Error logging progress: ", err)
	}
}

// createDirIfNotExist creates a directory at a given path, unless it already exists.
func createDirIfNotExist(dirPath string) {
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		log.Printf("Creating worker progress directory at: %v\n", dirPath)
		err := os.Mkdir(dirPath, 0777)
		if err != nil {
			fatal(err)
		}
	}
}

// printQueries prints out the body of Postgres queries used in Grafana, not including repeated parts.
// Can be easily modified to print time averages or moving averages for each entry in each array
func printQueries() {
	fmt.Printf("Size per bucket query: \n\n")
	for i := 0; i < NUM_FEE_BUCKETS-1; i++ {
		fmt.Printf("\"size_per_fee_bucket\"[%v] AS \"Num Txs with feerate: %v to %v sats/vbyte\",\n", i+1, FEE_BUCKET_VALUES[i], FEE_BUCKET_VALUES[i+1])
	}
}

// email sends an email to the recipient email with the recipient name, subject,
// and body.
func email(subject, body string) error {
	recipientEmails := os.Getenv("RECIPIENT_EMAILS")
	emailAddr := os.Getenv("EMAIL_ADDR")
	emailPw := os.Getenv("EMAIL_PASSWORD")
	log.Println(emailAddr, emailPw, recipientEmails)
	s, err := gomail.NewDialer("smtp.gmail.com", 587, emailAddr, emailPw).Dial()
	if err != nil {
		return err
	}

	emails := strings.Split(recipientEmails, ",")

	m := gomail.NewMessage()
	m.SetHeader("From", emailAddr)
	m.SetHeader("To", emails...)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", body)
	return gomail.Send(s, m)
}

// wrapper over log.Fatal to do other things before exit.
func fatal(v ...interface{}) {
	if SEND_EMAIL {
		body := fmt.Sprint(v...)
		email("Dashboard Process Failed", body)
	}

	log.Fatal("Shutdown caused by fatal error: ", v)

}
