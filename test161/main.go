package main

import (
	"fmt"
	"github.com/ops-class/test161"
	"os"
	"path"
)

var env *test161.TestEnvironment
var conf *ClientConf

func envInit() {
	var err error

	// Find and load .test161.conf.  Check the current directory, then $HOME

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

	if file == "" {
		printDefaultConf()
		os.Exit(1)
	}

	if conf, err = ClientConfFromFile(file); err != nil {
		fmt.Println("Error reading client configuration:", err)
		os.Exit(1)
	}

	if env, err = test161.NewEnvironment(conf.Test161Dir, nil); err != nil {
		fmt.Println("Error creating environment:", err)
		os.Exit(1)
	}

	env.RootDir = conf.RootDir
	env.OverlayRoot = conf.OverlayDir
}

func usage() {

	fmt.Println(`
    usage: test161  <command> <flags> <args>
 
           test161 run [-dry-run | -r] [sequential | -s] [-dependencies | -d] 
                       [-verbose | -v (whisper|quiet|loud*)] [-tag] <names>

           test161 submit <target> <commit>

           test161 list-targets [-remote | -r]

           test161 version

           test161 help for a detailed description
`)
}

func help() {
	usage()
	fmt.Println(`
    commands:
           'test161 run' runs a single target, or a collection of tests. Specify
           -dependencies to also run all tests' dependencies, which is done
           automatically for targets.  Tests may be specified by name,
           doublestar globbing, or by tag. If -tag is specified with a single
           positional argument, it is interpretted as tag.  This flag can be
           safely omitted as long there as there are no conflicts between tag
           and target name.

           Unless specified by -sequential, all output is interleaved with a summary
           at the end.  You can turn off the output lines with -v quiet, and hide
           everything except pass/fail with -v whisper. Specifying -dry-run will
           show you the tests that would be run, without running them.

           'test161' submit will create a submission for <target> and using commit
           <commit>.  This command will return a status, but will not block while
           grading.

           'test161 list-targets' will print a list of available targets.  Specifying
           -r will query the test161 server for this list.
	`)
}

func main() {
	exitcode := 2

	if len(os.Args) == 1 {
		usage()
	} else {
		// Get the sub-command
		if os.Args[1] == "help" {
			help()
		} else {
			// For the rest, we need a TestEnvironment
			envInit() // This might exit
			switch os.Args[1] {
			case "run":
				exitcode = doRun()
			case "submit":
				exitcode = doSubmit()
			case "list-targets":
				exitcode = doListTargets()
			case "version":
				fmt.Printf("test161 version: %v\n", test161.Version)
				exitcode = 0
			default:
				usage()
			}
		}
	}
	os.Exit(exitcode)
}
