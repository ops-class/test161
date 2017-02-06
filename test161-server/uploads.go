package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ops-class/test161"
	"mime/multipart"
	"net/http"
)

type UploadHandler interface {
	HandleFile(*multipart.FileHeader, *test161.UploadRequest) error
	FileComplete(error)
}

type UploadFileReader struct {
	Compressed bool
	LineChan   chan string
}

func NewUploadFileReader(gzipped bool, lineChan chan string) *UploadFileReader {
	return &UploadFileReader{
		Compressed: gzipped,
		LineChan:   lineChan,
	}
}

type UploadTypeManager interface {
	test161Server
	GetFileHandler() UploadHandler
}

func (handler *UploadFileReader) HandleFile(header *multipart.FileHeader) error {
	file, err := header.Open()
	if err != nil {
		return err
	}

	defer file.Close()

	var scanner *bufio.Scanner

	if handler.Compressed {
		if gz, err := gzip.NewReader(file); err != nil {
			return err
		} else {
			scanner = bufio.NewScanner(gz)
			defer gz.Close()
		}
	} else {
		scanner = bufio.NewScanner(file)
	}

	for scanner.Scan() {
		handler.LineChan <- scanner.Text()
	}

	return scanner.Err()
}

const MAX_FILE_DOWNLOAD = 50 * 1024 * 1024

// Upload manager/handlers. Currently we just have one (stats).
type UploadHandlerFactory func() UploadHandler

var uploadManagers map[int]UploadHandlerFactory

func initUploadManagers() {
	uploadManagers = make(map[int]UploadHandlerFactory)
	uploadManagers[test161.UPLOAD_TYPE_USAGE] = GetUsageStatHandler
}

func uploadFiles(w http.ResponseWriter, r *http.Request) {

	// The upload request has 2 pieces:
	//   1) A json string with user info so we can validate users
	//   2) A list of files and their contents
	//
	// If we are able to validate the users, we'll process the file contents.

	var req test161.UploadRequest

	if err := r.ParseMultipartForm(MAX_FILE_DOWNLOAD); err != nil {
		logger.Println("Error parsing multi-part form:", err)
		sendErrorCode(w, http.StatusBadRequest, err)
		return
	}

	if data, ok := r.MultipartForm.Value["request"]; !ok {
		logger.Println("request field not found")
		sendErrorCode(w, http.StatusBadRequest, errors.New("Missing form field: request"))
		return
	} else if len(data) != 1 {
		logger.Println("Invalid request field. Length != 1: ", data)
		sendErrorCode(w, http.StatusNotAcceptable, errors.New("Invalid request field. Length != 1"))
		return
	} else {
		json.Unmarshal([]byte(data[0]), &req)
		if _, err := req.Validate(serverEnv); err != nil {
			sendErrorCode(w, 422, err)
			return
		}
	}

	var handler UploadHandler

	if generator, ok := uploadManagers[req.UploadType]; !ok {
		sendErrorCode(w, http.StatusBadRequest, fmt.Errorf("Invalid upload type %v: ", req.UploadType))
		logger.Println("Invalid upload type request:", req.UploadType)
		return
	} else {
		handler = generator()
	}

	for fname, headers := range r.MultipartForm.File {
		fmt.Println("Processing", fname+"...")
		for _, fheader := range headers {
			err := handler.HandleFile(fheader, &req)
			handler.FileComplete(err)
		}
	}

	r.MultipartForm.RemoveAll()

	w.Header().Set("Content-Type", JsonHeader)
	w.WriteHeader(http.StatusOK)
}
