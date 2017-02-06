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
	"sync"
)

// Config args
var (
	configDebug bool
)

const CONF_FILE = ".test161.conf"
const SERVER = "https://test161.ops-class.org"

var CACHE_DIR = path.Join(os.Getenv("HOME"), ".test161/cache")
var KEYS_DIR = path.Join(os.Getenv("HOME"), ".test161/keys")
var USAGE_DIR = path.Join(os.Getenv("HOME"), ".test161/usage")
var USAGE_LOCK_FILE = path.Join(os.Getenv("HOME"), ".test161/usage/usage.lock")
var CUR_USAGE_LOCK_FILE = path.Join(os.Getenv("HOME"), ".test161/usage/current.lock")

type ClientConf struct {
	// These are now the only thing we put in the yaml file.
	// Everything else is inferred or set through environment
	// variables.
	Users []*test161.SubmissionUserInfo `yaml:"users"`

	// Test161Dir is optional, and it's usually inferred from source.
	Test161Dir string `yaml:"test161dir"`

	OverlayDir string `yaml:"-"` // Env
	Server     string `yaml:"-"` // Env override
	RootDir    string `yaml:"-"`
	SrcDir     string `yaml:"-"`
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

func isRootDir(p string) bool {
	reqs := []string{"kernel"}
	if err := testPath(p, "root", reqs); err != nil {
		return false
	}

	// Make sure it's executable (os.Stat follows links)
	if fi, err := os.Stat(path.Join(p, "kernel")); err != nil {
		return false
	} else {
		return !fi.IsDir() && (fi.Mode()&0111 > 0)
	}
}

func isSourceDir(path string) bool {
	reqs := []string{"kern", "userland", "mk"}
	err := testPath(path, "source", reqs)
	return err == nil
}

func isTest161Dir(path string) bool {
	reqs := []string{"commands", "targets", "tests"}
	err := testPath(path, "source", reqs)
	return err == nil
}

// Seach for a directory starting at 'start', popping up the tree until 'home'.
// This used fn() to determine if the directory is a match.
func searchForDir(start, home string, fn func(string) bool) string {
	prev := ""
	dir := start
	for strings.HasPrefix(dir, home) && prev != dir {
		if fn(dir) {
			return dir
		}
		prev = dir
		dir = filepath.Dir(dir)
	}
	return ""
}

// Infer the test161 configuration from the current working directory.
// This replaces most of .test161.conf.
func inferConf() (*ClientConf, error) {
	var err error
	src, root, test := "", "", ""
	cwd, home := "", ""

	if cwd, err = os.Getwd(); err != nil {
		return nil, fmt.Errorf("Cannot retrieve current working directory: %v", err)
	}
	if cwd, err = filepath.Abs(cwd); err != nil {
		// Unlikely
		return nil, fmt.Errorf("Cannot determine absolute path of cwd: %v", err)
	}
	if home = os.Getenv("HOME"); home == "" {
		// Unlikely
		return nil, errors.New("test161 requires your $HOME environment variable to be set")
	}
	if !strings.HasPrefix(cwd, home) {
		// Unlikely, but more likely
		return nil, errors.New("Trying to run test161 outside of your $HOME directory?")
	}

	// Search for all of the directories we need, starting with cwd
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		src = searchForDir(cwd, home, isSourceDir)
	}()

	go func() {
		defer wg.Done()
		root = searchForDir(cwd, home, isRootDir)
	}()

	wg.Wait()

	testJoin := func(base, dir string, fn func(string) bool) string {
		temp := path.Join(base, dir)
		if fn(temp) {
			return temp
		} else {
			return ""
		}
	}

	// If we couldn't find root or source from the CWD, we may be able to
	// find one if we know the other.
	if src == "" && root != "" {
		src = testJoin(root, ".src", pathExists)
	} else if root == "" && src != "" {
		root = testJoin(src, ".root", pathExists)
	}

	// If we have source and not test161, we can hopefully find test161 in source
	// or root. If not, it may be configured in .test161.conf, which gets checked
	// later.
	if test == "" && src != "" {
		test = testJoin(src, "test161", isTest161Dir)
	}
	if test == "" && root != "" {
		test = testJoin(root, "test161", isTest161Dir)
	}

	// Clean up symlinks so the paths are cleaner when printed
	if root != "" {
		if root, err = filepath.EvalSymlinks(root); err != nil {
			return nil, fmt.Errorf("An error occurred evaluating symlinks in your root path (%v): %v", root, err)
		}
	}
	if src != "" {
		if src, err = filepath.EvalSymlinks(src); err != nil {
			return nil, fmt.Errorf("An error occurred evaluating symlinks in your source path (%v): %v", src, err)
		}
	}
	if test != "" {
		if test, err = filepath.EvalSymlinks(test); err != nil {
			return nil, fmt.Errorf("An error occurred evaluating symlinks in your test161 path (%v): %v", test, err)
		}
	}

	inferred := &ClientConf{
		Server:     SERVER,
		Users:      make([]*test161.SubmissionUserInfo, 0),
		RootDir:    root,
		SrcDir:     src,
		Test161Dir: test,
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

// Validate all required paths in the config file. If source or root are set,
// we know they have the right structure, but they may be required and not
// present. The overlay dir (env variable) may not be valid. The test161 dir
// may not be valid if there's an override in the .test161.conf file.
func (conf *ClientConf) checkPaths(cmd *test161Command) (err error) {

	err = nil

	if conf.RootDir == "" && cmd.reqRoot {
		return errors.New("Unable to execute command: test161 cannot determine your root directory from the CWD.")
	}

	if conf.SrcDir == "" && cmd.reqSource {
		return errors.New("Unable to execute command: test161 cannot determine your source directory from the CWD.")
	}

	if conf.Test161Dir == "" && cmd.reqTests {
		return errors.New("Unable to execute command: test161 cannot determine your test161 directory from the CWD.")
	} else if cmd.reqTests && !isTest161Dir(conf.Test161Dir) {
		return fmt.Errorf(`"%v" is not a valid test161 directory.`, conf.Test161Dir)
	}

	// Overlay directory
	if len(conf.OverlayDir) > 0 {
		if err = testPath(conf.OverlayDir, "OverlayDirectory", []string{"asst1"}); err != nil {
			return
		}
	}

	return
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

	if clientConf.SrcDir == "" {
		return 0
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

func setTest161Dir(test161dir string) error {

	if !pathExists(test161dir) {
		return errors.New("Unable to set test161 directory: directory not found")
	}
	if !isTest161Dir(test161dir) {
		return errors.New("Unable to set test161 directory: invalid test161 directory")
	}

	conf, err := getConfFromFileSafe()
	if err != nil {
		return err
	}

	conf.Test161Dir = test161dir
	return ClientConfToFile(conf)
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
		case "test161dir":
			if len(args) != 2 {
				usage()
				return 1
			}
			err = setTest161Dir(args[1])

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

// Initialize the cache, key, and usage directories in HOME/.test161
func init() {
	dirs := []string{
		CACHE_DIR, KEYS_DIR, USAGE_DIR,
	}

	for _, dirname := range dirs {
		if _, err := os.Stat(dirname); err != nil {
			if err := os.MkdirAll(dirname, 0770); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating '%v': %v\n", dirname, err)
				os.Exit(1)
			}
		}
	}
}
