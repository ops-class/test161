package main

import (
	"encoding/json"
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
	TestDir    string `yaml:"test_dir"`
	TargetDir  string `yaml:"target_dir"`
	MaxTests   uint   `yaml:"max_tests"`
	Database   string `yaml:"dbname"`
	DBServer   string `yaml:"dbsever"`
	DBUser     string `yaml:"dbuser"`
	DBPassword string `yaml:"dbpw"`
	DBTimeout  uint   `yaml:"dbtimeout"`
}

const CONF_FILE = ".test161-server.conf"

var defaultConfig = &SubmissionServerConfig{
	CacheDir:   "/var/cache/test161/builds",
	TestDir:    "../fixtures/tests/nocycle",
	TargetDir:  "../fixtures/targets",
	MaxTests:   0,
	Database:   "test161",
	DBServer:   "localhost:27017",
	DBUser:     "",
	DBPassword: "",
	DBTimeout:  10,
}

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

	var request test161.SubmissionRequest

	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))

	if err != nil {
		panic(err)
	}

	if err := r.Body.Close(); err != nil {
		panic(err)
	}

	if err := json.Unmarshal(body, &request); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(422) // unprocessable entity

		log.Printf("Error unmarshalling submission request.\nError: %v\nRequest: ", err, string(body))

		if err := json.NewEncoder(w).Encode(err); err != nil {
			log.Println("Error encoding error:", err)
			return
		}
		return
	}

	// TODO: Target verification.  We should only allow certain targets in certain
	// windows of time.

	// Make sure we can create the submission
	submission, errs := test161.NewSubmission(&request, serverEnv)
	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(errs); err != nil {
			log.Println("Error encoding error:", err)
			return
		}
		return
	}

	w.WriteHeader(http.StatusCreated)

	// Run it!
	go submission.Run()
}

func apiUsage(w http.ResponseWriter, r *http.Request) {

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
		log.Println("Using default server configuration")
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
	env, err := test161.NewEnvironment(s.conf.TestDir, s.conf.TargetDir)
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
	log.Fatal(http.ListenAndServe(":4000", NewRouter()))
}

func (s *SubmissionServer) Stop() {
	test161.StopManager()
	serverEnv.Persistence.Close()
}
