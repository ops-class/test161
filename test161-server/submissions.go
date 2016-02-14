package main

import (
	"encoding/json"
	"github.com/ops-class/test161"
	"gopkg.in/mgo.v2"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

// Submission Manager for test161

// Environment for running test161 submissions
var serverEnv *test161.TestEnvironment

// Environment config
type SubmissionServerConfig struct {
	CacheDir  string `yaml:"cache_dir"`
	TestDir   string `yaml:"test_dir"`
	TargetDir string `yaml:"target_dir"`
	MaxTests  uint   `yaml:"max_tests"`
	Database  string `yaml:"database"`
}

var defaultConfig = &SubmissionServerConfig{
	CacheDir:  "/var/cache/test161/builds",
	TestDir:   "../fixtures/tests/nocycle",
	TargetDir: "../fixtures/targets",
	MaxTests:  0,
	Database:  "test161",
}

type SubmissionServer struct {
	config *SubmissionServerConfig
}

func NewSubmissionServer() (test161Server, error) {
	s := &SubmissionServer{
		config: defaultConfig,
	}

	if err := setUpEnvironment(); err != nil {
		return nil, err
	}

	return s, nil
}

// listTargets return all targets available to submit to
func listTargets(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)

	list := serverEnv.TargetList()

	if err := json.NewEncoder(w).Encode(list); err != nil {
		panic(err)
	}
	return
}

// createSubmission accepts POST requests
func createSubmission(w http.ResponseWriter, r *http.Request) {

	var submission test161.SubmissionRequest

	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))

	if err != nil {
		panic(err)
	}

	if err := r.Body.Close(); err != nil {
		panic(err)
	}

	if err := json.Unmarshal(body, &submission); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(422) // unprocessable entity
		if err := json.NewEncoder(w).Encode(err); err != nil {
			panic(err)
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(submission); err != nil {
		panic(err)
	}
}

func apiUsage(w http.ResponseWriter, r *http.Request) {

}

func setUpEnvironment() error {
	// Submission environment
	env, err := test161.NewEnvironment(defaultConfig.TestDir, defaultConfig.TargetDir)
	if err != nil {
		return err
	}

	env.CacheDir = defaultConfig.CacheDir

	// MongoDB connection
	mongoTestDialInfo := &mgo.DialInfo{
		Addrs:    []string{"localhost:27017"},
		Timeout:  60 * time.Second,
		Database: defaultConfig.Database,
		Username: "",
		Password: "",
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
	test161.SetManagerCapacity(defaultConfig.MaxTests)
	test161.StartManager()
	log.Fatal(http.ListenAndServe(":4000", NewRouter()))
}

func (s *SubmissionServer) Stop() {
	test161.StopManager()
	serverEnv.Persistence.Close()
}
