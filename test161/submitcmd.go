package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ops-class/test161"
	"github.com/parnurzeal/gorequest"
	"net/http"
	"os"
	"strings"
)

var (
	submitCommit     string
	submitRef        string
	submitTargetName string
)

const SubmitMsg = `
The CSE 421/521 Collaboration Guidelines for this assignment are as follows:%v

Your submission will receive an estimated score of %v/%v points.

Do you certify that you have followed the collaboration guidelines and wish to submit now?
`

func localSubmitTest(req *test161.SubmissionRequest) (score, available uint, errs []error) {

	score = 0
	available = 0

	var submission *test161.Submission

	env.Persistence = &ConsolePersistence{}
	submission, errs = test161.NewSubmission(req, env)
	if len(errs) > 0 {
		return
	}

	test161.SetManagerCapacity(0)
	test161.StartManager()
	defer test161.StopManager()

	if err := submission.Run(); err != nil {
		errs = []error{err}
		return
	}

	score = submission.Score
	available = submission.PointsAvailable

	return
}

func doSubmit() (exitcode int) {

	collabMsg := ""
	exitcode = 1

	// Parse args
	if targetInfo, err := getSubmitArgs(); err != nil {
		printRunError(err)
		return
	} else {
		collabMsg = targetInfo.CollabMsg
	}

	req := &test161.SubmissionRequest{
		Target:        submitTargetName,
		Users:         clientConf.Users,
		Repository:    clientConf.Repository,
		CommitID:      submitCommit,
		ClientVersion: test161.Version,
	}

	score, avail := uint(0), uint(0)

	// Local build
	var errs []error
	score, avail, errs = localSubmitTest(req)
	if len(errs) > 0 {
		printRunErrors(errs)
		return
	}

	// Don't bother proceeding if no points earned
	if score == 0 && avail > 0 {
		fmt.Println("\nNo points will be earned for this submission, cancelling submission.")
		return
	}

	// Show score and collab policy, and give them a chance to cancel
	fmt.Printf(SubmitMsg, collabMsg, score, avail)
	reader := bufio.NewReader(os.Stdin)
	for {
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)
		if text == "no" {
			fmt.Println("\nSubmission request cancelled\n")
			return
		} else if text == "yes" {
			break
		} else {
			fmt.Println("\nPlease answer 'yes' or 'no'")
		}
	}

	// Submit
	endpoint := clientConf.Server + "/api-v1/submit"
	remoteRequest := gorequest.New()

	if reqbytes, err := json.Marshal(req); err != nil {
		printRunError(err)
		return
	} else {
		resp, body, errs := remoteRequest.Post(
			endpoint).
			Send(string(reqbytes)).
			End()

		if len(errs) > 0 {
			printRunErrors(errs)
		} else {
			if resp.StatusCode == http.StatusCreated {
				fmt.Println("\nYour submission has been created and is being processed by the test161 server\n")
				exitcode = 0
			} else if resp.StatusCode == http.StatusNotAcceptable {
				fmt.Println("Unable to accept your submission, test161 is out-of-date.  Please update test161 and resubmit")
			} else {
				printRunError(fmt.Errorf("\nThe server could not process your request: %v. \nData: %v\n",
					resp.Status, body))
			}
		}
	}

	return
}

func getRemoteTargetAndValidate(name string) (*test161.TargetListItem, error) {
	var ourVersion *test161.Target
	var serverVersion *test161.TargetListItem
	var ok bool
	ourVersion, ok = env.Targets[name]
	if !ok {
		return nil, fmt.Errorf("Target '%v' does not exist locally.  Please update your os161 sources.", name)
	}

	// Verfiy it exists on the sever, and is up to date
	list, errs := getRemoteTargets()
	if len(errs) > 0 {
		return nil, errs[0]
	}

	for _, target := range list.Targets {
		if target.Name == submitTargetName {
			// Verify that the targets are actually the same
			if target.FileHash != ourVersion.FileHash {
				return nil, fmt.Errorf("Target '%v' is out of sync with the server version.  Please update your os161 sources", name)
			}
			serverVersion = target
			break
		}
	}

	if serverVersion == nil {
		return nil, fmt.Errorf("The target '%v' does not exist on the remote sever", name)
	}

	return serverVersion, nil
}

func getSubmitArgs() (*test161.TargetListItem, error) {

	args := os.Args[2:]

	debug := false

	if len(args) > 0 && args[0] == "-debug" {
		args = args[1:]
		debug = true
	}

	if len(args) == 0 {
		return nil, errors.New("test161 submit: Missing target name. run test161 help for detailed usage")
	} else if len(args) > 2 {
		return nil, errors.New("test161 submit: Too many arguments. run test161 help for detailed usage")
	}

	submitTargetName = args[0]

	// Get remote target
	serverVersion, err := getRemoteTargetAndValidate(submitTargetName)
	if err != nil {
		return nil, err
	}

	// Get the commit ID and ref
	git, err := gitRepoFromDir(clientConf.SrcDir, debug)
	if err != nil {
		return nil, err
	}

	if !git.canSubmit() {
		// This prints its own message
		return nil, errors.New("Unable to submit")
	}

	commit, ref := "", ""

	// Try to get a commit id/ref
	if len(args) == 2 {
		treeish := args[1]
		commit, ref, err = git.commitFromTreeish(treeish, debug)
	} else {
		commit, ref, err = git.commitFromHEAD(debug)
	}

	if err != nil {
		return nil, err
	}

	clientConf.Repository = git.remoteURL
	submitCommit = commit
	submitRef = ref

	return serverVersion, nil
}
