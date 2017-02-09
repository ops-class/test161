package main

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"github.com/kardianos/osext"
	"github.com/ops-class/test161"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"syscall"
	"time"
)

// 1 MB
const MAX_FILE_SIZE = 1 * 1024 * 1024

// Actual number is +1 for current
const MAX_NUM_FILES = 20

const USAGE_FILE_PREFIX = "usage"
const USAGE_FILE_EXT = ".json.gz"
const USAGE_FILE = USAGE_FILE_PREFIX + USAGE_FILE_EXT

// This is less than the server accepts, so we have room to play with.
const MAX_UPLOAD_SIZE = 10 * 1024 * 1024

// Log a single usage entry for either a test group, target, or tag
func logUsageStat(tg *test161.TestGroup, tagTarget string, start, end time.Time) error {
	// Check if we should save off the old one. If the uploader is running,
	// don't bother.
	err := lockFile(USAGE_LOCK_FILE, false)
	if err == nil {
		checkMoveUsgaeFile(false)
		unlockFile(USAGE_LOCK_FILE)
	}

	// Lock the current usage file while log the entry. We set the wait flag,
	// so this should block until finished.
	err = lockFile(CUR_USAGE_LOCK_FILE, true)
	if err != nil {
		return err
	}
	defer unlockFile(CUR_USAGE_LOCK_FILE)

	// It's possible the user info isn't filled in when the tests are run. The
	// server can fill in the blanks with the auth info.
	users := make([]string, 0)
	for _, user := range clientConf.Users {
		users = append(users, user.Email)
	}

	stat := test161.NewTestGroupUsageStat(users, tagTarget, tg, start, end)
	err = saveUsageStat(stat)
	if err != nil {
		printRunError(err)
	}
	return err
}

// Serialize the stat and append to the current usage file
func saveUsageStat(stat *test161.UsageStat) error {
	fname := getCurUsageFilename()
	f, err := os.OpenFile(fname, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}

	defer f.Close()

	if j, err := stat.JSON(); err != nil {
		return err
	} else {
		gz := gzip.NewWriter(f)
		writer := bufio.NewWriter(gz)
		_, err = writer.WriteString(j + "\n")
		writer.Flush()
		gz.Close()
		return err
	}
}

// Check if we should move the usage file due to space concerns.
// If force is true, do it regardless of size -- and don't try
// to prune anything.
func checkMoveUsgaeFile(force bool) {
	source := getCurUsageFilename()
	dest := path.Join(USAGE_DIR, fmt.Sprintf("%v_%011d%v", USAGE_FILE_PREFIX,
		time.Now().Unix(), USAGE_FILE_EXT))

	fi, err := os.Stat(source)
	if err == nil && (force || fi.Size() >= MAX_FILE_SIZE) {
		os.Rename(source, dest)
		if !force {
			checkPruneUsageFiles()
		}
	}
}

// See if we hit our space limit, and prune old files if we did.
func checkPruneUsageFiles() {
	files, err := getAllUsageFiles()
	if err != nil {
		return
	}

	if len(files) > MAX_NUM_FILES {
		numDeletes := len(files) - MAX_NUM_FILES
		// The timestamps are 0-padded, so sorting ascending will put the
		// oldest files first. But, we want to ignore the current usage file.
		sort.Strings(files)
		pos := 0
		for pos < len(files) && numDeletes > 0 {
			file := files[pos]
			os.Remove(file)
			numDeletes -= 1
			pos += 1
		}
	}
}

// Spawn the uploader in the background.
func runTest161Uploader() {
	// Try to use the version of test161 that is currently running.
	exename, _ := osext.Executable()
	if len(exename) == 0 {
		exename = "test161"
	}

	cmd := exec.Command(exename, "upload-usage")
	cmd.Start()
}

func handleUploadError(err error) {
	// TODO: Log this somewhere
	fmt.Println(err)
	return
}

func handleUploadErrors(errors []error) {
	// TODO: Log this somewhere
	fmt.Println(errors)
	return
}

type fileSizeInfo struct {
	name string
	size int64
}

func getFileSizes(files []string) ([]*fileSizeInfo, error) {

	res := make([]*fileSizeInfo, 0)

	for _, f := range files {
		if fi, err := os.Stat(f); err != nil {
			return nil, err
		} else {
			res = append(res, &fileSizeInfo{f, fi.Size()})
		}
	}

	return res, nil
}

// Get the next group of files to upload, so that the total of their size is
// less than or equal to maxChunkSize.
// We use a simple greedy approach rather than optimally packing the upload.
func nextUploadChunk(files []*fileSizeInfo, curPos int, maxChunkSize int64) ([]string, int) {
	if curPos >= len(files) {
		return nil, curPos
	}

	res := make([]string, 0)
	chunkSize := int64(0)

	for curPos < len(files) {
		fi := files[curPos]
		if chunkSize+fi.size <= maxChunkSize {
			res = append(res, files[curPos].name)
			chunkSize += fi.size
		} else {
			break
		}
		curPos += 1
	}
	return res, curPos
}

// Upload a group of usage files to the test161 server.
func uploadStatFiles(files []string) []error {

	pos := 0
	allFileInfo, err := getFileSizes(files)
	if err != nil {
		return []error{err}
	}

	for pos < len(files) {
		chunk, newPos := nextUploadChunk(allFileInfo, pos, MAX_UPLOAD_SIZE)
		if newPos == pos {
			// We have a file that's too big
			// TODO: log this too
			pos += 1
			continue
		}

		pos = newPos

		// Upload this batch of files
		pr := NewPostRequest(ApiEndpointUpload)
		pr.SetType(PostTypeMultipart)

		req := &test161.UploadRequest{
			UploadType: test161.UPLOAD_TYPE_USAGE,
			Users:      clientConf.Users,
		}

		if err := pr.QueueJSON(req, "request"); err != nil {
			return []error{err}
		}

		for _, f := range chunk {
			pr.QueueFile(f, "usage")
		}

		resp, body, errs := pr.Submit()

		if len(errs) > 0 {
			return errs
		} else if resp.StatusCode != http.StatusOK {
			return []error{
				fmt.Errorf("Upload failed. Status code: %v Msg: %v", resp.StatusCode, body),
			}
		} else {
			for _, f := range chunk {
				os.Remove(f)
			}
		}
	}

	return nil
}

// Entry point for running test161 upload-usage
func doUploadUsage() int {
	// First, acquire the usage lock so we don't compete with another
	// uploader process.
	if err := lockFile(USAGE_LOCK_FILE, true); err != nil {
		// Already running
		return 1
	}
	defer unlockFile(USAGE_LOCK_FILE)

	// Next, move the current usage file out of the way so we don't compte with
	// 'test161 run ...' logging a new stat.
	if err := moveCurrentUsage(); err != nil {
		// Current usage file in use
		return 1
	}

	usageFiles, err := getAllUsageFiles()
	if err != nil {
		handleUploadError(err)
		return 1
	}

	// OK, we can upload now without competition from uploading or logging.
	if len(usageFiles) > 0 {
		if errs := uploadStatFiles(usageFiles); len(errs) > 0 {
			handleUploadErrors(errs)
			return 1
		}
	}
	return 0
}

// Make a copy of the current usage file so we can get out of the way and let
// the user continue to run tests.
func moveCurrentUsage() error {
	fname := getCurUsageFilename()
	if _, err := os.Stat(fname); err != nil {
		return nil
	}

	if lockErr := lockFile(CUR_USAGE_LOCK_FILE, true); lockErr != nil {
		return lockErr
	}
	defer unlockFile(CUR_USAGE_LOCK_FILE)

	// We'll just move the current file so it gets queued
	checkMoveUsgaeFile(true)

	return nil
}

// Get a list of all usage files in the usage directory, EXCEPT FOR
// the current usage file.
func getAllUsageFiles() ([]string, error) {
	files, err := ioutil.ReadDir(USAGE_DIR)
	if err != nil {
		return nil, err
	}

	usageFiles := make([]string, 0)

	for _, file := range files {
		name := file.Name()
		// Adding "_" will skip the current usage file.
		if strings.HasPrefix(name, USAGE_FILE_PREFIX+"_") && strings.HasSuffix(name, USAGE_FILE_EXT) {

			usageFiles = append(usageFiles, path.Join(USAGE_DIR, name))
		}
	}

	return usageFiles, nil
}

// flock() a file in the filesystem
func lockFile(fileName string, block bool) error {

	file, err := os.OpenFile(fileName, os.O_CREATE+os.O_APPEND, 0666)
	if err != nil {
		return err
	}

	op := syscall.LOCK_EX
	if block {
		op += syscall.LOCK_NB
	}

	fd := file.Fd()
	return syscall.Flock(int(fd), op)
}

// un-flock() a file in the filesystem
func unlockFile(fileName string) error {
	file, err := os.OpenFile(fileName, os.O_CREATE+os.O_APPEND, 0666)
	if err != nil {
		return err
	}

	fd := file.Fd()
	return syscall.Flock(int(fd), syscall.LOCK_UN)
}

func getCurUsageFilename() string {
	return path.Join(USAGE_DIR, USAGE_FILE)
}
