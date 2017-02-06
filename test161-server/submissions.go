package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ops-class/test161"
	"gopkg.in/mgo.v2"
	yaml "gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"time"
)

// Submission Manager for test161

// Environment for running test161 submissions
var serverEnv *test161.TestEnvironment
var submissionMgr *test161.SubmissionManager

// Environment config
type SubmissionServerConfig struct {
	CacheDir   string                 `yaml:"cachedir"`
	Test161Dir string                 `yaml:"test161dir`
	OverlayDir string                 `yaml:"overlaydir"`
	KeyDir     string                 `yaml:"keydir"`
	UsageDir   string                 `yaml:"usagedir"`
	MaxTests   uint                   `yaml:"max_tests"`
	Database   string                 `yaml:"dbname"`
	DBServer   string                 `yaml:"dbsever"`
	DBUser     string                 `yaml:"dbuser"`
	DBPassword string                 `yaml:"dbpw"`
	DBTimeout  uint                   `yaml:"dbtimeout"`
	APIPort    uint                   `yaml:"api_port"`
	MinClient  test161.ProgramVersion `yaml:"min_client"`
}

const CONF_FILE = ".test161-server.conf"

var defaultConfig = &SubmissionServerConfig{
	CacheDir:   "/var/cache/test161/builds",
	Test161Dir: "../fixtures/",
	MaxTests:   0,
	Database:   "test161",
	DBServer:   "localhost:27017",
	DBUser:     "",
	DBPassword: "",
	DBTimeout:  10,
	APIPort:    4000,
	MinClient:  test161.ProgramVersion{0, 0, 0},
}

var logger = log.New(os.Stderr, "test161-server: ", log.LstdFlags)

type SubmissionServer struct {
	conf *SubmissionServerConfig
}

func NewSubmissionServer() (test161Server, error) {

	conf, err := loadServerConfig()
	if err != nil {
		return nil, err
	}

	s := &SubmissionServer{
		conf: conf,
	}

	if err := s.setUpEnvironment(); err != nil {
		return nil, err
	}

	return s, nil
}

var minClientVer test161.ProgramVersion

// listTargets return all targets available to submit to
func listTargets(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", JsonHeader)
	w.WriteHeader(http.StatusOK)

	list := serverEnv.TargetList()

	if err := json.NewEncoder(w).Encode(list); err != nil {
		logger.Println("Error encoding target list:", err)
	}
}

func submissionFromHttp(w http.ResponseWriter, r *http.Request, validateOnly bool) *test161.SubmissionRequest {
	var request test161.SubmissionRequest

	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1*1024*1024))
	if err != nil {
		logger.Println("Error reading web request:", err)
		w.WriteHeader(http.StatusBadRequest)
		return nil
	}

	if err := r.Body.Close(); err != nil {
		logger.Println("Error closing submission request body:", err)
		w.WriteHeader(http.StatusBadRequest)
	}

	if !validateOnly {
		logger.Println("Submission Request:", string(body))
	} else {
		logger.Println("Validation Request:", string(body))
	}

	if err := json.Unmarshal(body, &request); err != nil {
		w.Header().Set("Content-Type", JsonHeader)
		w.WriteHeader(http.StatusBadRequest)

		logger.Printf("Error unmarshalling submission request. Error: %v\nRequest: ", err, string(body))
		if err := json.NewEncoder(w).Encode(err); err != nil {
			logger.Println("Encoding error:", err)
		}
		return nil
	}

	// Check the client's version and make sure it's not too old
	if request.ClientVersion.CompareTo(minClientVer) < 0 {
		logger.Printf("Old request (version %v)\n", request.ClientVersion)
		sendErrorCode(w, http.StatusNotAcceptable, errors.New(
			"test161 version too old, test161-server requires version "+minClientVer.String()))
		return nil
	}

	if submissionMgr.Status() == test161.SM_NOT_ACCEPTING {
		// We're trying to shut down
		logger.Println("Rejecting due to SM_NOT_ACCEPTING")
		sendErrorCode(w, http.StatusServiceUnavailable,
			errors.New("The submission server is currently not accepting new submissions"))
		return nil
	}

	return &request
}

// createSubmission accepts POST requests
func createSubmission(w http.ResponseWriter, r *http.Request) {

	request := submissionFromHttp(w, r, false)
	if request == nil {
		return
	}

	// Make sure we can create the submission.  This checks for everything but run errors.
	submission, errs := test161.NewSubmission(request, serverEnv)
	if len(errs) > 0 {
		w.Header().Set("Content-Type", JsonHeader)
		w.WriteHeader(422) // unprocessable entity

		// Marhalling a slice of arrays doesn't work, so we'll send back strings.
		errorStrings := []string{}
		for _, e := range errs {
			errorStrings = append(errorStrings, fmt.Sprintf("%v", e))
		}

		if err := json.NewEncoder(w).Encode(errorStrings); err != nil {
			logger.Println("Encoding error:", err)
		}
		return
	}

	w.WriteHeader(http.StatusCreated)

	// Run it!
	go func() {
		if err := submissionMgr.Run(submission); err != nil {
			logger.Println("Error running submission:", err)
		}
	}()
}

// validate accepts POST requests
func validateSubmission(w http.ResponseWriter, r *http.Request) {

	request := submissionFromHttp(w, r, true)
	if request == nil {
		return
	}

	if _, err := request.Validate(serverEnv); err != nil {
		// Unprocessable entity
		sendErrorCode(w, 422, err)
		return
	}

	keyInfo := request.CheckUserKeys(serverEnv)
	w.Header().Set("Content-Type", JsonHeader)
	w.WriteHeader(http.StatusOK)

	if len(keyInfo) > 0 {
		if err := json.NewEncoder(w).Encode(keyInfo); err != nil {
			logger.Println("Encoding error (Validate Response):", err)
		}
	}
}

// getStats returns the current manager statistics
func getStats(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", JsonHeader)
	w.WriteHeader(http.StatusOK)

	stats := submissionMgr.CombinedStats()

	if err := json.NewEncoder(w).Encode(stats); err != nil {
		logger.Println("Error encoding stats:", err)
	}
}

func apiUsage(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, `<html><body>See <a href="https://github.com/ops-class/test161">the ops-class test161 GitHub page </a> for API and usage</body></html>`)
}

type KeygenRequest struct {
	Email string
	Token string
}

// Generate a public/private key pair for a particular user
func keygen(w http.ResponseWriter, r *http.Request) {
	var request KeygenRequest

	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 2*1024))
	if err != nil {
		logger.Println("Error reading web request:", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := r.Body.Close(); err != nil {
		logger.Println("Error closing submission request body:", err)
		w.WriteHeader(http.StatusBadRequest)
	}

	if err := json.Unmarshal(body, &request); err != nil {
		logger.Printf("Error unmarshalling keygen request. Error: %v\nRequest: ", err, string(body))
		sendErrorCode(w, http.StatusBadRequest, errors.New("Error unmarshalling keygen request."))
		return
	}

	key, err := test161.KeyGen(request.Email, request.Token, serverEnv)
	if err != nil {
		// Unprocessable entity
		sendErrorCode(w, 422, err)
	} else {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, key)
	}

}

func loadServerConfig() (*SubmissionServerConfig, error) {

	// Check current directory, but fall back to home directory
	search := []string{
		CONF_FILE,
		path.Join(os.Getenv("HOME"), CONF_FILE),
	}

	file := ""

	for _, f := range search {
		if _, err := os.Stat(f); err == nil {
			file = f
			break
		}
	}

	// Use defaults
	if file == "" {
		logger.Println("Using default server configuration")
		// TODO: Spit out the default config
		return defaultConfig, nil
	}

	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	conf := &SubmissionServerConfig{}
	err = yaml.Unmarshal(data, conf)

	if err != nil {
		return nil, err
	}

	return conf, nil
}

func (s *SubmissionServer) setUpEnvironment() error {
	// MongoDB connection
	mongoTestDialInfo := &mgo.DialInfo{
		Username: s.conf.DBUser,
		Password: s.conf.DBPassword,
		Database: s.conf.Database,
		Addrs:    []string{s.conf.DBServer},
		Timeout:  time.Duration(s.conf.DBTimeout) * time.Second,
	}

	mongo, err := test161.NewMongoPersistence(mongoTestDialInfo)
	if err != nil {
		return err
	}

	// Submission environment
	env, err := test161.NewEnvironment(s.conf.Test161Dir, mongo)
	if err != nil {
		return err
	}

	env.CacheDir = s.conf.CacheDir
	env.OverlayRoot = s.conf.OverlayDir
	env.KeyDir = s.conf.KeyDir
	env.Log = logger

	// Set the min client version where the handler can access it
	minClientVer = s.conf.MinClient
	usageFailDir = s.conf.UsageDir

	fmt.Println("Min client ver:", minClientVer)

	// OK, we're good to go
	serverEnv = env
	submissionMgr = test161.NewSubmissionManager(serverEnv)

	return nil
}

func (s *SubmissionServer) Start() {
	// Kick off test161 submission server
	test161.SetManagerCapacity(s.conf.MaxTests)
	test161.StartManager()

	// Init upload handlers
	initUploadManagers()

	// Finally, start listening for internal API requests
	logger.Fatal(http.ListenAndServe(fmt.Sprintf(":%v", s.conf.APIPort), NewRouter()))
}

func (s *SubmissionServer) Stop() {
	test161.StopManager()
	serverEnv.Persistence.Close()
}
