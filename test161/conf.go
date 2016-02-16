package main

import (
	"fmt"
)

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
	Test161Dir string                        `yaml:"test161dir"`
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

func printDefaultConf() {
	fmt.Println(`
test161 needs a configuration file to run.  Create a '.test161.conf' in your $HOME directory or the directory
where you plan to run test161. The following is an example .test161.conf file that you can modify with your
specific information. Note, the conf file is in yaml format (so no tabs please), and --- must be the first 
line in the file.

(Example .test161.conf)
---
rootdir: /path/to/os161/root
test161dir: /path/to/os161/src/test161
server: https://test161.ops-class.org
repository: git@your-remote-git-repo.os161.git
users:
  - email: "your-email@bufalo.edu"
    token: "your-token (from test161.ops-class.org"
  - email: "your-email@bufalo.edu"
    token: "your-token (from test161.ops-class.org"
`)
}
