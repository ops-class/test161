package main

import (
	"errors"
	"fmt"
	"github.com/imdario/mergo"
	"github.com/ops-class/test161"
	yaml "gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
)

const CONF_FILE = ".test161.conf"
const SERVER = "https://test161.ops-class.org"

const (
	OptionStrict = iota
	OptionLenient
)

type ClientConf struct {
	Repository string                        `yaml:"repository"`
	Server     string                        `yaml:"server"`
	RootDir    string                        `yaml:"rootdir"`
	SrcDir     string                        `yaml:"srcdir"`
	Test161Dir string                        `yaml:"test161dir"`
	OverlayDir string                        `yaml:"overlaydir"`
	Users      []*test161.SubmissionUserInfo `yaml:"users"`
}

func (conf *ClientConf) mergeConf(other *ClientConf) error {
	return mergo.Map(conf, other)
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

func ClientConfToFile(conf *ClientConf, file string) error {
	text, err := yaml.Marshal(conf)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(file, []byte(text), 0664)
}

func printDefaultConf() {
	fmt.Println(`
test161 needs a configuration file to run.  Create a '.test161.conf' in your $HOME directory or the directory
where you plan to run test161. The following is an example .test161.conf file that you can modify with your
specific information. Note, the conf file is in yaml format (so no tabs please), and --- must be the first 
line in the file.

*If you run test161 in your source or root directory, you only need to include user information.

(Example .test161.conf)
---
rootdir: /path/to/os161/root
test161dir: /path/to/os161/src/test161
srcdir: /path/to/src
server: https://test161.ops-class.org
repository: git@your-remote-git-repo.os161.git
users:
  - email: "your-email@buffalo.edu"
    token: "your-token (from test161.ops-class.org)"
  - email: "your-email@buffalo.edu"
    token: "your-token (from test161.ops-class.org)"
`)
}

func inferConf() (*ClientConf, error) {
	var err error

	src, root, remote := "", "", ""

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("Cannot retrieve current working directory: %v", err)
	}

	// Are we in root? Check for .src and kernel
	if _, err = os.Stat(".src"); err == nil {
		if _, err = os.Stat("kernel"); err == nil {
			src = path.Join(cwd, ".src")
			root = cwd
		}
	}

	// Are we in src? Check for kern/ and .root
	if root == "" {
		if _, err = os.Stat(".root"); err == nil {
			if _, err = os.Stat("kern"); err == nil {
				src = cwd
				root = path.Join(cwd, ".root")
			}
		}
	}

	if root == "" || src == "" {
		return nil, errors.New("test161 must be run in either your os161 root or source directory")
	}

	if root, err = filepath.EvalSymlinks(root); err != nil {
		return nil, fmt.Errorf("An error occurred evaluating symlinks in your root path (%v): %v", root, err)
	}

	if src, err = filepath.EvalSymlinks(src); err != nil {
		return nil, fmt.Errorf("An error occurred evaluating symlinks in your root path (%v): %v", src, err)
	}

	// Skip repo name until we need it (submit)

	// It's possible they have a weird setup, so we'll do everything but the
	// remote if we get an error above.
	inferred := &ClientConf{
		Repository: remote,
		Server:     SERVER,
		Users:      make([]*test161.SubmissionUserInfo, 0),
		RootDir:    root,
		SrcDir:     src,
		Test161Dir: path.Join(src, "test161"),
		OverlayDir: "",
	}

	return inferred, err
}

func pathExists(p string) bool {
	if _, err := os.Stat(p); err == nil {
		return true
	} else {
		return false
	}
}

// Test a single path, looking for the existence of the path and the mustContain
// elements in the path.
func testPath(p, desc string, mustContain []string) error {
	if len(p) > 0 {
		if !pathExists(p) {
			return fmt.Errorf(`%v "%v" does not exist`, desc, p)
		} else {
			for _, elem := range mustContain {
				if !pathExists(path.Join(p, elem)) {
					return fmt.Errorf(`%v "%v" does not contain "%v"`, desc, p, elem)
				}
			}
		}
	} else {
		return fmt.Errorf("%v must be specified", desc)
	}

	return nil
}

func (conf *ClientConf) checkPaths() (err error) {

	// Root Directory
	if err = testPath(conf.RootDir, "Root Directory", []string{"kernel"}); err != nil {
		return
	}

	// test161 Directory
	if err = testPath(conf.Test161Dir, "test161 Directory", []string{"targets", "tests", "commands"}); err != nil {
		return
	}

	// Source Directory
	if len(conf.SrcDir) == 0 {
		conf.SrcDir = path.Dir(conf.Test161Dir)
		if err = testPath(conf.SrcDir, "Source Directory", []string{"kern", "mk"}); err != nil {
			return
		}
	}

	// Overlay directory
	if len(conf.OverlayDir) > 0 {
		if err = testPath(conf.OverlayDir, "OverlayDirectory", []string{"asst1"}); err != nil {
			return
		}
	}

	err = nil
	return
}
