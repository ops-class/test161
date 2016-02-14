package main

import (
	"github.com/ops-class/test161"
	yaml "gopkg.in/yaml.v2"
	"io/ioutil"
)

const CONF_FILE = ".test161.conf"

type ClientConf struct {
	Repository string                        `yaml:"repository"`
	Token      string                        `yaml:"token"`
	Server     string                        `yaml:"server"`
	Users      []*test161.SubmissionUserInfo `yaml:"users"`
	RootDir    string                        `yaml:"rootdir"`
	TargetDir  string                        `yaml:"targetdir"`
	TestDir    string                        `yaml:"testdir"`
}

func ClientConfFromFile(file string) (*ClientConf, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	return ClientConfFromString(string(data))
}

func ClientConfFromString(text string) (*ClientConf, error) {
	conf := &ClientConf{}
	err := yaml.Unmarshal([]byte(text), conf)

	if err != nil {
		return nil, err
	}

	return conf, nil
}
