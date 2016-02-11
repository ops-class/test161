package main

import (
	yaml "gopkg.in/yaml.v2"
	"io/ioutil"
)

const CONF_FILE = ".test161.conf"

type ClientConf struct {
	Repo      string   `yaml:"repo"`
	Token     string   `yaml:"token"`
	Ids       []string `yaml:"ids"`
	RootDir   string   `yaml:"rootdir"`
	TargetDir string   `yaml:"targetdir"`
	TestDir   string   `yaml:"testdir"`
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
