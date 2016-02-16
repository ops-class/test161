package main

import (
	"encoding/json"
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

// Environment config
type SubmissionServerConfig struct {
	CacheDir   string `yaml:"cache_dir"`
	Test161Dir string `yaml:"test161dir`
	MaxTests   uint   `yaml:"max_tests"`
	Database   string `yaml:"dbname"`
	DBServer   string `yaml:"dbsever"`
	DBUser     string `yaml:"dbuser"`
	DBPassword string `yaml:"dbpw"`
	DBTimeout  uint   `yaml:"dbtimeout"`
	APIPort    uint   `yaml:"api_port"`
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

const JsonHeader = "application/json; charset=UTF-8"

// listTargets return all targets available to submit to
func listTargets(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", JsonHeader)
	w.WriteHeader(http.StatusOK)

	list := serverEnv.TargetList()

	if err := json.NewEncoder(w).Encode(list); err != nil {
		logger.Println("Error encoding target list:", err)
	}
}

// createSubmission accepts POST requests
func createSubmission(w http.ResponseWriter, r *http.Request) {

	var request test161.SubmissionRequest

	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1*1024*1024))
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
		w.Header().Set("Content-Type", JsonHeader)
		w.WriteHeader(http.StatusBadRequest)

		logger.Printf("Error unmarshalling submission request. Error: %v\nRequest: ", err, string(body))
		if err := json.NewEncoder(w).Encode(err); err != nil {
			logger.Println("Encoding error:", err)
		}
		return
	}

	// Make sure we can create the submission.  This checks for everything but run errors.
	submission, errs := test161.NewSubmission(&request, serverEnv)
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
		if runerr := submission.Run(); err != nil {
			logger.Println("Error running submission:", runerr)
		}
	}()
}

func apiUsage(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, `<html><body>See <a href="https://github.com/ops-class/test161">the ops-class test161 GitHub page </a> for API and usage</body></html>`)
}

func loadServerConfig() (*SubmissionServerConfig, error) {

	// Check current directory, but fall back to home directory
	search := []string{
		CONF_FILE,
		path.Join(os.Getenv("HOME"), CONF_FILE),
	}

	file := ""

	for _, f := range search {
		if _, err2 := os.Stat(f); err2 == nil {
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

	// Submission environment
	env, err := test161.NewEnvironment(s.conf.Test161Dir)
	if err != nil {
		return err
	}

	env.CacheDir = s.conf.CacheDir

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
	env.Persistence = mongo

	// OK, we're good to go
	serverEnv = env

	return nil
}

func (s *SubmissionServer) Start() {
	test161.SetManagerCapacity(s.conf.MaxTests)
	test161.StartManager()
	logger.Fatal(http.ListenAndServe(fmt.Sprintf(":%v", s.conf.APIPort), NewRouter()))
}

func (s *SubmissionServer) Stop() {
	test161.StopManager()
	serverEnv.Persistence.Close()
}
