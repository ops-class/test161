package main

import (
	"fmt"
	"github.com/ops-class/test161"
	"os"
)

var env *test161.TestEnvironment
var clientConf *ClientConf

func envInit() {
	var err error

	// Get our bearings
	clientConf, err = inferConf()
	if err != nil || clientConf == nil {
		fmt.Fprintf(os.Stderr, "\nUnable to determine your test161 configuration:\n%v\n\n", err)
		os.Exit(1)
	}

	// Now load their actual conf file to get user info.
	confFile, err := getConfFromFile()
	if err != nil {
		// An error here means we couldn't load the file, either bad yaml or I/O problem
		fmt.Fprintf(os.Stderr, "An error occurred reading your %v file: \n", CONF_FILE, err)
		os.Exit(1)
	}

	// OK if confFile is nil, but they won't be able to submit.
	if confFile != nil {
		// The user info is all we store in .test161.conf at this point
		clientConf.Users = confFile.Users
	}

	// Environment variable overrides
	clientConf.OverlayDir = os.Getenv("TEST161_OVERLAY")
	if server := os.Getenv("TEST161_SERVER"); server != "" {
		clientConf.Server = server
	}

	// Test all the paths before trying to load the environment. Only the overlay
	// should really be a problem since we're figuring everything else out from
	// the cwd.
	if err = clientConf.checkPaths(); err != nil {
		fmt.Fprintf(os.Stderr, "\nThe following paths are incorrect in your configuration: %v\n\n", err)
		os.Exit(1)
	}

	// Lastly, create the acutal test environment, loading the targets, commands, and
	// tests.
	if env, err = test161.NewEnvironment(clientConf.Test161Dir, nil); err != nil {
		fmt.Fprintf(os.Stderr, "\nUnable to create your test161 test environment:\n%v\n\n", err)
		os.Exit(1)
	}

	env.RootDir = clientConf.RootDir
	env.OverlayRoot = clientConf.OverlayDir
}

func usage() {
	fmt.Fprintf(os.Stdout, `usage:
    test161  <command> <flags> <args>

    test161 run [-dry-run | -d] [-explain | -x] [sequential | -s]
                [-no-dependencies | -n] [-verbose | -v (whisper|quiet|loud*)]
                [-tag] <names>

    test161 submit [-debug] [-verify] [-no-cache] <target> <commit>

    test161 list (targets|tags|tests) [-remote | -r]

    test161 config [-debug] [(add-user|del-user|change-token)] <username> <token>

    test161 version

    test161 help for a detailed commands description
`)
}

func doHelp() int {
	usage()
	fmt.Fprintf(os.Stdout, `
Commands Description:

'test161 run' runs a single target, or a group of tests. By default, all
dependencies for the test group will also be run. For single tests and tags,
specifying -no-dependencies will only run the test and tags given on the command
line.

Specififying Tests: Individual tests may be specified by name, doublestar
globbing, or by tag. Because naming conflicts could arise between tags and
targets, adding the -tag flag forces test161 to interpet a single positional
argument as a tag. This flag can be safely omitted as long there as there are no
conflicts between tag and target name.

Output: Unless specified by -sequential, all output is interleaved with a
summary at the end.  You can disable test output lines with -v quiet, and hide
everything except pass/fail with -v whisper. Specifying -dry-run will show you
the tests that would be run, without running them. Similarly, -explain will show
you more detailed information about the tests and what they expect, without
running them. This option is very useful when writing your own tests.


'test161 submit' creates a submission for <target> on the test161.ops-class.org
server. This command will return a status, but will not block while evaluating
the target on the server.

Specifying a Commit: 'test161 submit' needs to send a Git commit id to the
server so that it can run your kernel. If omitted, test161 will use the commit
id corresponding the tip of your current branch. The submit command will also
recognize sha commit ids, branches, and tags. For example, the following are all
valid:
		 test161 submit asst1 origin/master    # origin/master
		 test161 submit asst1 asst1            # asst1 is a tag

Debugging: Adding the -verify flag will validate the submission will by checking
for local and remote issues, without submitting. This is useful for debugging
username and token, deployment key, and repository issues. Adding -debug will
print the git commands that submit uses to determine the status of your
repository.  Adding -no-cache will clone the repo locally instead of using a
previously cached copy.


'test161 list' prints a variety of useful information. 'test161 list targets'
shows the local targets available to test161; adding -r will show the remote
targets instead. 'test161 list tags' shows a listing of tags with the tests in
each tag. 'test161 list tests' lists all tests available to test161 along with
their descriptions.


'test161 config' is used to view and change test161 configuration. When run with
no arguments, this command shows test161 path, user, and Git repository
information. Add -debug to  see the Git commands that are run.

User Configuration: 'test161 config' can also edit user/token data. There are
three commands available to modify user configuration:

	  'test161 config add-user <email> <token>'
	  'test161 config del-user <email>'
	  'test161 config change-token <email> <new-token>'
`)

	return 0
}

type test161Command struct {
	cmd    func() int
	reqEnv bool
}

var cmdTable = map[string]*test161Command{
	"run":     &test161Command{doRun, true},
	"submit":  &test161Command{doSubmit, true},
	"list":    &test161Command{doListCommand, true},
	"config":  &test161Command{doConfig, true},
	"version": &test161Command{doVersion, false},
	"help":    &test161Command{doHelp, false},
}

func doVersion() int {
	fmt.Printf("test161 version: %v\n", test161.Version)
	return 0
}

func main() {
	exitcode := 2

	if len(os.Args) == 1 {
		usage()
	} else {
		// Get the sub-command
		if cmd, ok := cmdTable[os.Args[1]]; ok {
			if cmd.reqEnv {
				envInit() // This might exit
			}
			exitcode = cmd.cmd()
		} else {
			fmt.Fprintf(os.Stderr, "\n\"%v\" is not a reconized test161 command", os.Args[1])
			usage()
		}
	}
	os.Exit(exitcode)
}
