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
	"os/exec"
	"regexp"
	"strings"
)

var (
	submitCommit     string
	submitTargetName string
)

const submit_msg = `
As a reminder, the CSE 421/521 Collaboration Policy...

Your submission will receive an estimated score of %v/%v points.

Do you wish to submit now? (yes to continue)

`

const LocalBuild bool = false

func localSubmitTest(req *test161.SubmissionRequest) (score, available uint, errs []error) {

	score = 0
	available = 0

	var submission *test161.Submission

	submission, errs = test161.NewSubmission(req, env)
	if len(errs) > 0 {
		return
	}

	env.Persistence = &ConsolePersistence{}

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

func doSubmit() {

	// Parse args
	if err := getSubmitArgs(); err != nil {
		printRunError(err)
		return
	}

	req := &test161.SubmissionRequest{
		Target:     submitTargetName,
		Users:      conf.Users,
		Repository: conf.Repository,
		CommitID:   submitCommit,
	}

	score, avail := uint(0), uint(0)

	if LocalBuild {
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
	}

	// Show score and collab policy, and give them a chance to cancel
	fmt.Printf(submit_msg, score, avail)
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')

	if text != "yes\n" {
		fmt.Println("\nSubmission request cancelled\n")
		return
	}

	// Submit
	endpoint := conf.Server + "/api-v1/submit"
	remoteRequest := gorequest.New()

	fmt.Println("\nContacting", conf.Server)

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
			} else {
				printRunError(fmt.Errorf("\nThe server could not process your request: %v. \nData: %v\n",
					resp.Status, body))
			}
		}
	}
}

func getSubmitArgs() error {

	args := os.Args[2:]

	if len(args) == 0 {
		return errors.New("test161 submit: Missing target name. run test161 help for detailed usage")
	} else if len(args) > 2 {
		return errors.New("test161 submit: Too many arguments. run test161 help for detailed usage")
	}

	submitTargetName = args[0]
	// Verfiy it exists on the sever, and is up to date

	list, errs := getRemoteTargets()
	if len(errs) > 0 {
		return errs[0]
	}

	var ok bool = false
	for _, target := range list.Targets {
		if target.Name == submitTargetName {
			// TODO: Verify the hash
			ok = true
			break
		}
	}

	if !ok {
		return fmt.Errorf("The target '%v' does not exist on the remote sever", submitTargetName)
	}

	// Try to get a commit id
	if len(args) == 2 {
		// Minimally, it needs to be a hex string
		if ok, err := regexp.MatchString("^[0-9a-f]+$", args[1]); err != nil {
			return err
		} else if !ok {
			return errors.New("test161 submit: Invalid commit ID")
		}
		submitCommit = args[1]
	} else {
		// Get HEAD
		cmd := exec.Command("git", "rev-parse", "HEAD")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("Error reading HEAD: %v. Are you in your source directory?", err)
		} else if len(output) == 0 {
			return fmt.Errorf("git rev-parse HEAD returned no output.  Unable to get commit id from HEAD.")
		} else {
			lines := strings.Split(string(output), "\n")
			if ok, err := regexp.MatchString("^[0-9a-f]+$", lines[0]); err != nil {
				return err
			} else if !ok {
				return errors.New("test161 submit: Invalid commit ID in HEAD?")
			} else {
				submitCommit = lines[0]
			}
		}
	}

	return nil
}
