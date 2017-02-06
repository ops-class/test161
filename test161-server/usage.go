package main

import (
	"encoding/json"
	"fmt"
	"github.com/ops-class/test161"
	"io"
	"mime/multipart"
	"os"
	"path"
	"time"
)

var usageFailDir string

func getFailedUsageFileName() string {
	res := path.Join(usageFailDir, fmt.Sprintf("usage_%v.json.gz", time.Now().Unix()))
	return res
}

type UsageStatsFileHandler struct {
	hasError bool
	header   *multipart.FileHeader
}

func (handler *UsageStatsFileHandler) HandleFile(header *multipart.FileHeader,
	req *test161.UploadRequest) error {

	// We'll defer grunt work to upload reader and just collect inflated lines.
	lineChan := make(chan string)
	reader := NewUploadFileReader(true, lineChan)
	handler.header = header
	handler.hasError = false

	// We'll replace the user info with what comes in the validated request.
	users := make([]string, 0)
	for _, u := range req.Users {
		users = append(users, u.Email)
	}

	go func() {
		for line := range lineChan {
			// Each line is a single usage log
			var usageStat test161.UsageStat

			if err := json.Unmarshal([]byte(line), &usageStat); err != nil {
				handler.hasError = true
				logger.Println("Invalid usage JSON:", line)
			} else {
				usageStat.Users = users
				if err = usageStat.Persist(serverEnv); err != nil {
					logger.Println("Error saving stat:", err)
				}
			}
		}
	}()

	return reader.HandleFile(header)
}

func (handler *UsageStatsFileHandler) tryCopyFile() {

	file, err := handler.header.Open()
	if err != nil {
		logger.Println("Failed to open the file for reading")
	}
	defer file.Close()

	outFile := getFailedUsageFileName()
	out, err := os.Create(outFile)
	if err != nil {
		logger.Printf("Failed to open '%v' file for writing.\n", outFile)
		return
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	if err != nil {
		logger.Println("Failed to copy file:", err)
	}
}

func (handler *UsageStatsFileHandler) FileComplete(err error) {
	if err != nil || handler.hasError {
		handler.tryCopyFile()
	}
}

// Generator function for usage stats uploads
func GetUsageStatHandler() UploadHandler {
	return &UsageStatsFileHandler{}
}
