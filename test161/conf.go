package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/ops-class/test161"
	yaml "gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Config args
var (
	configDebug bool
)

const CONF_FILE = ".test161.conf"
const SERVER = "https://test161.ops-class.org"

var CACHE_DIR = path.Join(os.Getenv("HOME"), ".test161/cache")
var KEYS_DIR = path.Join(os.Getenv("HOME"), ".test161/keys")

type ClientConf struct {
	// This is now the only thing we put in the yaml file.
	// Everything else is inferred or set through environment
	// variables.
	Users []*test161.SubmissionUserInfo `yaml:"users"`

	OverlayDir string `yaml:"-"` // Env
	Server     string `yaml:"-"` // Env override
	RootDir    string `yaml:"-"`
	SrcDir     string `yaml:"-"`
	Test161Dir string `yaml:"-"`
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

func ClientConfToFile(conf *ClientConf) error {
	file := path.Join(os.Getenv("HOME"), CONF_FILE)
	text, err := yaml.Marshal(conf)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(file, []byte(text), 0664)
	if err != nil {
		return fmt.Errorf("Error writing client file: %v", err)
	}
	return nil
}

func printDefaultConf() {
	fmt.Println(`
test161 needs a configuration file in order to submit to the server.  Create a '.test161.conf' 
in your $HOME directory, or the directory where you plan to run test161. The following is an 
example .test161.conf file that you can modify with your group information. Note: the conf file 
is in yaml format (so no tabs please).

Alternatively, use 'test161 config add-user' to add user information from the command line.
 
(Example .test161.conf)
---
users:
  - email: "your-email@buffalo.edu"
    token: "your-token (from test161.ops-class.org)"
  - email: "your-email@buffalo.edu"
    token: "your-token (from test161.ops-class.org)"
`)
}

func isRootDir(path string) bool {
	reqs := []string{"kernel", ".src"}
	err := testPath(path, "root", reqs)
	return err == nil
}

func isSourceDir(path string) bool {
	reqs := []string{"kern", ".root", "userland", "mk"}
	err := testPath(path, "source", reqs)
	return err == nil
}

// Infer the required test161 configuration from the current working directory.
// This replaces most of .test161.conf.
func inferConf() (*ClientConf, error) {
	var err error

	src, root := "", ""

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("Cannot retrieve current working directory: %v", err)
	}

	// Are we in root?
	if isRootDir(cwd) {
		root = cwd
		src = path.Join(cwd, ".src")
	} else {
		// Are we somewhere in src? Walk backwards until we find out.
		home := os.Getenv("HOME")
		if home == "" {
			// Unlikely
			return nil, errors.New("test161 requires your $HOME environment variable to be set")
		}

		dir, err := filepath.Abs(cwd)
		if err != nil {
			// Unlikely
			return nil, fmt.Errorf("Cannot determine absolute path of cwd: %v", err)
		}

		if !strings.HasPrefix(dir, home) {
			// Unlikely, but more likely
			return nil, errors.New("Trying to run test161 outside of your $HOME directory?")
		}

		prev := ""
		for strings.HasPrefix(dir, home) && prev != dir {
			if isSourceDir(dir) {
				src = dir
				root = path.Join(dir, ".root")
				break
			}
			prev = dir
			dir = filepath.Dir(dir)
		}
	}

	if root == "" || src == "" {
		return nil, errors.New("test161 must be run in either your OS/161 root directory, or inside your OS/161 source tree")
	}

	if root, err = filepath.EvalSymlinks(root); err != nil {
		return nil, fmt.Errorf("An error occurred evaluating symlinks in your root path (%v): %v", root, err)
	}

	if src, err = filepath.EvalSymlinks(src); err != nil {
		return nil, fmt.Errorf("An error occurred evaluating symlinks in your root path (%v): %v", src, err)
	}

	// Skip repo name until we need it (submit)

	inferred := &ClientConf{
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
			return fmt.Errorf(`%v: "%v" does not exist`, desc, p)
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

// Validate all paths in the config file. Now that we're figuring out the configuration
// from the current directory, this shouldn't fail, except for possibly the overlay directory.
func (conf *ClientConf) checkPaths() (err error) {

	if conf == nil {
		fmt.Println("Conf is nil???")
	}

	// Root Directory
	if err = testPath(conf.RootDir, "Root Directory", []string{"kernel"}); err != nil {
		return
	}

	// Source Directory
	if len(conf.SrcDir) == 0 {
		conf.SrcDir = path.Dir(conf.Test161Dir)
		if err = testPath(conf.SrcDir, "Source Directory", []string{"kern", "mk", "test161"}); err != nil {
			return
		}
	}

	// test161 Directory
	if err = testPath(conf.Test161Dir, "test161 Directory", []string{"targets", "tests", "commands"}); err != nil {
		return
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

// Get a key to use for git ssh commands. We follow what the server does and pick the
// first one that exists.
func (conf *ClientConf) getKeyFile() string {
	for _, u := range conf.Users {
		keyfile := path.Join(KEYS_DIR, u.Email, "id_rsa")
		if _, err := os.Stat(keyfile); err == nil {
			return keyfile
		}
	}
	return ""
}

// test161 config (-debug]
func doShowConf() int {

	fmt.Println("\nPath Configuration:")
	fmt.Println("--------------------------------")
	if clientConf.OverlayDir != "" {
		fmt.Println("Overlay Directory       :", clientConf.OverlayDir)
	}
	fmt.Println("Root Directory          :", clientConf.RootDir)
	fmt.Println("OS/161 Source Directory :", clientConf.SrcDir)
	fmt.Println("test161 Directory       :", clientConf.Test161Dir)
	fmt.Println("test161 Server          :", clientConf.Server)

	fmt.Println("\n\nUser Configuration:")
	fmt.Println("--------------------------------")
	for _, u := range clientConf.Users {
		fmt.Println(" - User   :", u.Email)
		fmt.Println("   Token  :", u.Token)
	}

	// Infer git info and print it
	git, err := gitRepoFromDir(clientConf.SrcDir, configDebug)
	if err != nil {
		fmt.Println(err)
		return 1
	}

	// Print git info
	fmt.Println("\n\nGit Repository Detail:")
	fmt.Println("--------------------------------")
	fmt.Println("Directory     :", git.dir)
	fmt.Println("Remote Name   :", git.remoteName)
	fmt.Println("Remote Ref    :", git.remoteRef)
	fmt.Println("Remote URL    :", git.remoteURL)
	if git.localRef == "HEAD" {
		fmt.Println("Local Ref     :", "(detached HEAD)")
	} else {
		fmt.Println("Local Ref     :", git.localRef)
	}

	if dirty, err := git.isLocalDirty(configDebug); err != nil {
		fmt.Println("Local Status  :", err)
	} else if !dirty {
		fmt.Println("Local Status  : clean")
	} else {
		fmt.Println("Local Status  : dirty")
	}

	fmt.Printf("Remote Status : checking...")

	if configDebug {
		// We replace the line above, but that doesn't work when we're debugging
		fmt.Println()
	}

	// Try the deploy key, but fall back to local authentication
	// in case things aren't set up yet, or the key changed.
	if ok, err := git.isRemoteUpToDate(configDebug, TryDeployKey); err != nil {
		fmt.Println("\rRemote Status :", "unknown     ")
	} else if ok {
		fmt.Println("\rRemote Status : up-to-date  ")
	} else {
		fmt.Println("\rRemote Status : out-of-sync  ")
	}
	fmt.Println()

	return 0
}

// Read $HOME/CONFIG_FILE
func getConfFromFile() (*ClientConf, error) {

	conf := &ClientConf{}
	var err error

	// We now only search HOME, though this is subject to change
	search := []string{
		path.Join(os.Getenv("HOME"), CONF_FILE),
	}

	file := ""
	for _, f := range search {
		if _, staterr := os.Stat(f); staterr == nil {
			file = f
			break
		}
	}

	if file == "" {
		return nil, nil
	} else if info, err := os.Stat(file); err != nil || info.Size() == 0 {
		return nil, nil
	}

	if conf, err = ClientConfFromFile(file); err != nil {
		return nil, err
	}

	return conf, nil
}

func getConfFromFileSafe() (*ClientConf, error) {
	conf, err := getConfFromFile()
	if err != nil {
		return conf, err
	}
	// In case .test161.conf doesn't exist...
	if conf == nil {
		conf = &ClientConf{
			Users: make([]*test161.SubmissionUserInfo, 0),
		}
	}
	return conf, nil
}

// test161 config add-user
func addUser(email, token string) error {
	if email == "" {
		return errors.New("Must specify an email in 'test161 config add-user`")
	} else if token == "" {
		return errors.New("Must specify a token in 'test161 config add-user`")
	}

	conf, err := getConfFromFileSafe()
	if err != nil {
		return err
	}

	for _, u := range conf.Users {
		if u.Email == email {
			return fmt.Errorf(`User "%v" already exists.`, email)
		}
	}
	conf.Users = append(conf.Users, &test161.SubmissionUserInfo{
		Email: email,
		Token: token,
	})
	return ClientConfToFile(conf)
}

// test161 config del-user
func delUser(email string) error {
	if email == "" {
		return errors.New("Must specify an email in 'test161 config del-user`")
	}

	conf, err := getConfFromFileSafe()
	if err != nil {
		return err
	}

	for i, u := range conf.Users {
		if u.Email == email {
			conf.Users = append(conf.Users[:i], conf.Users[i+1:]...)
			return ClientConfToFile(conf)
		}
	}
	return fmt.Errorf(`User "%v" does not exist in %v`, email, CONF_FILE)
}

// test161 config change-token
func changeToken(email, token string) error {
	if email == "" {
		return errors.New("Must specify an email in 'test161 config change-token`")
	} else if token == "" {
		return errors.New("Must specify a token in 'test161 config change-token`")
	}

	conf, err := getConfFromFileSafe()
	if err != nil {
		return err
	}

	for _, u := range conf.Users {
		if u.Email == email {
			u.Token = token
			return ClientConfToFile(conf)
		}
	}

	return fmt.Errorf(`User "%v" not found in %v`, email, CONF_FILE)
}

// test161 config (add-user | del-user | change-token)
func doConfig() int {

	configFlags := flag.NewFlagSet("test161 config", flag.ExitOnError)
	configFlags.Usage = usage

	configFlags.BoolVar(&configDebug, "debug", false, "")
	configFlags.Parse(os.Args[2:]) // this may exit

	args := configFlags.Args()

	var err error

	if len(args) == 0 {
		return doShowConf()
	} else {
		switch args[0] {
		case "add-user":
			if len(args) != 3 {
				usage()
				return 1
			}
			err = addUser(args[1], args[2])
		case "del-user":
			if len(args) != 2 {
				usage()
				return 1
			}
			err = delUser(args[1])
		case "change-token":
			if len(args) != 3 {
				usage()
				return 1
			}
			err = changeToken(args[1], args[2])
		default:
			fmt.Fprintf(os.Stderr, "Invalid option for config\n")
			usage()
			return 1
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		} else {
			return 0
		}
	}
}
