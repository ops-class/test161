package main

import (
	"fmt"
	"github.com/ops-class/test161"
	"os"
	"path"
)

var env *test161.TestEnvironment
var clientConf *ClientConf

const configOpt = OptionLenient

func getConfFromFile() (*ClientConf, error) {

	conf := &ClientConf{}
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
		return nil, nil
	} else if info, err := os.Stat(file); err != nil || info.Size() == 0 {
		return nil, nil
	}

	if conf, err = ClientConfFromFile(file); err != nil {
		return nil, err
	}

	return conf, nil
}

func envInit() {

	// Try to infer the config file first
	inferred, err := inferConf()
	if err != nil && configOpt == OptionStrict {
		fmt.Println("Unable to determine your test161 configuration:", err)
		os.Exit(1)
	}

	// Now load their actual conf file
	confFile, err := getConfFromFile()
	if err != nil {
		// An error here means we couldn't load the file, either bad yaml or I/O problem
		fmt.Printf("An error occurred reading your %v file: \n", CONF_FILE, err)
		os.Exit(1)
	}

	if inferred != nil && confFile != nil {
		// Merge the two, favoring the inferred file
		if err = inferred.mergeConf(confFile); err != nil {
			fmt.Println("test161 was unable to merge your default configuration:", err)
			os.Exit(1)
		}
		clientConf = inferred
	} else if inferred != nil {
		clientConf = inferred
	} else {
		clientConf = confFile
	}

	// Test all the paths before trying to load the environment
	if err = clientConf.checkPaths(); err != nil {
		fmt.Println()
		fmt.Println("The following paths are incorrect in your configuration:", err)
		fmt.Println()
		printDefaultConf()
		os.Exit(1)
	}

	if env, err = test161.NewEnvironment(clientConf.Test161Dir, nil); err != nil {
		fmt.Println("Unable to create your test161 test environment:", err)
		os.Exit(1)
	}

	env.RootDir = clientConf.RootDir
	env.OverlayRoot = clientConf.OverlayDir

	// The server in the conf file has to override the default one for local testing
	if confFile.Server != "" {
		clientConf.Server = confFile.Server
	}

}

func usage() {

	fmt.Println(`
    usage: test161  <command> <flags> <args>
 
           test161 run [-dry-run | -r] [-explain | -x] [sequential | -s]
                       [-dependencies | -d] [-verbose | -v (whisper|quiet|loud*)]
                       [-tag] <names>

           test161 explain [-tag] <names>

           test161 submit [-debug] [-verify] <target> <commit>

           test161 list (targets|tags|tests|conf) [-debug] [-remote | -r]

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
           show you the tests that would be run, without running them. Similarly,
           -explain will give you more detailed information about the tests and
           what they expect, without running them. This option is very useful when
           writing your own tests.

           'test161' submit will create a submission for <target> and using commit
           <commit>.  This command will return a status, but will not block while
           grading. Specifying -verify will verify that the submission will be accepted
           by the server, without submitting it. This is useful for debugging username
           and token issues. Adding -debug will print the git commands that submit uses
           to determine the status of your repository.

           'test161 list targets' will print a list of available targets.  Specifying
           -r will query the test161 server for this list.

           'test161 list (tags|tests)' prints the list of local tags or tests.

           'test161 list conf [-debug] prints the configuration and Git repository
           information. Adding -debug will print the individual Git commands used.
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
			case "list":
				exitcode = doListCommand()
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
