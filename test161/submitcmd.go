package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/ops-class/test161"
	"github.com/parnurzeal/gorequest"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strings"
)

var (
	submitDebug      bool
	submitVerfiy     bool
	submitNoCache    bool
	submitCommit     string
	submitRef        string
	submitTargetName string
)

const SubmitMsg = `
The CSE 421/521 Collaboration Guidelines for this assignment are as follows:%v

Your submission will receive an estimated score of %v/%v points.

Do you certify that you have followed the collaboration guidelines and wish to submit now?
`

// Run the submission locally, but as close to how the server would do it
// as possible
func localSubmitTest(req *test161.SubmissionRequest) (score, available uint, errs []error) {

	score = 0
	available = 0

	var submission *test161.Submission

	// Cache builds for performance, unless we're told not to
	if !submitNoCache {
		env.CacheDir = CACHE_DIR
	}

	env.KeyDir = KEYS_DIR
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

	printRunSummary(submission.Tests, VERBOSE_LOUD, true)

	score = submission.Score
	available = submission.PointsAvailable

	return
}

func getYesOrNo() string {
	reader := bufio.NewReader(os.Stdin)
	for {
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)
		if text == "no" || text == "yes" {
			return text
		} else {
			fmt.Println("\nPlease answer 'yes' or 'no'")
		}
	}
}

// test161 submit ...
func doSubmit() (exitcode int) {

	collabMsg := ""
	exitcode = 1

	// Early sanity checks
	if len(clientConf.Users) == 0 {
		printDefaultConf()
		return
	}

	// Check the version of git to figure out if we can even build locally
	if ok, err := checkGitVersionAndComplain(); err != nil {
		err = fmt.Errorf("Unable to check Git version: %v", err)
		return
	} else if !ok {
		return
	}

	// Parse args and verify the target
	if targetInfo, err := getSubmitArgs(); err != nil {
		printRunError(err)
		return
	} else {
		collabMsg = targetInfo.CollabMsg
	}

	req := &test161.SubmissionRequest{
		Target:        submitTargetName,
		Users:         clientConf.Users,
		Repository:    clientConf.git.remoteURL,
		CommitID:      submitCommit,
		ClientVersion: test161.Version,
	}

	// Get the current hash of our test161 private key
	for _, user := range req.Users {
		user.KeyHash = getKeyHash(user.Email)
	}

	// Validate before running locally (and install their keys)
	if err := validateUsers(req); err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		if submitVerfiy {
			return
		}
	} else if submitVerfiy {
		// If only -verify, we're done.
		exitcode = 0
		fmt.Println("OK")
		return
	}

	// We've verified what we can. Time to test things locally before submission.
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
		fmt.Println("No points will be earned for this submission, cancelling submission.")
		return
	}

	// Show score and collab policy, and give them a chance to cancel
	fmt.Printf(SubmitMsg, collabMsg, score, avail)
	if text := getYesOrNo(); text == "no" {
		fmt.Println("\nSubmission request cancelled\n")
		return
	}

	// Confirm the users
	for i, u := range req.Users {
		fmt.Printf("\n(%v of %v): You are submitting on behalf of %v. Is this correct?\n",
			i+1, len(req.Users), u.Email)
		if text := getYesOrNo(); text == "no" {
			fmt.Println("\nSubmission request cancelled\n")
			return
		}
	}

	// Let the server know what we think we're going to get
	req.EstimatedScore = score

	// Finally, submit
	if err := submit(req); err == nil {
		fmt.Println("Your submission has been created and is being processed by the test161 server")
		exitcode = 0
	} else {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}

	return
}

// Validate the user info on the server, and update the users' private keys
// that are returned by the server. Fail if the user hasn't set up a key yet.
func validateUsers(req *test161.SubmissionRequest) error {
	body, err := submitOrValidate(req, true)
	if err != nil {
		return err
	}

	// All keys are up-to-date and exist
	if len(body) == 0 {
		return nil
	}

	// Handle the response from the server, specifically, handle
	// the test161 private keys that are returned.
	keyData := make([]*test161.RequestKeyResonse, 0)
	if err := json.Unmarshal([]byte(body), &keyData); err != nil {
		return fmt.Errorf("Unable to parse server response (validate): %v", err)
	}

	emptyCount := 0

	for _, data := range keyData {
		if data.Key != "" {
			studentDir := path.Join(KEYS_DIR, data.User)
			if _, err := os.Stat(studentDir); err != nil {
				err = os.Mkdir(studentDir, 0770)
				if err != nil {
					return fmt.Errorf("Error creating user's key directory: %v", err)
				}
			}
			file := path.Join(KEYS_DIR, data.User, "id_rsa")
			if err := ioutil.WriteFile(file, []byte(data.Key), 0600); err != nil {
				return fmt.Errorf("Error creating private key: %v", err)
			}
		} else {
			emptyCount += 1
			fmt.Fprintf(os.Stderr, "Warning: No test161 key exists for", data.User)
		}
	}

	// Check if no keys have been set up
	if emptyCount == len(clientConf.Users) && emptyCount > 0 {
		return errors.New(`test161 requires you to add a test161 deployment key to your Git repository. To create a new key pair, 
login to https://test161.ops-class.org and go to your settings page.`)
	}

	return nil
}

func submit(req *test161.SubmissionRequest) error {
	_, err := submitOrValidate(req, false)
	return err
}

// Return true if OK, false otherwise
func submitOrValidate(req *test161.SubmissionRequest, validateOnly bool) (string, error) {

	endpoint := clientConf.Server
	if validateOnly {
		endpoint += "/api-v1/validate"
	} else {
		endpoint += "/api-v1/submit"
	}

	remoteRequest := gorequest.New()
	if reqbytes, err := json.Marshal(req); err != nil {
		return "", err
	} else {
		resp, body, errs := remoteRequest.Post(
			endpoint).
			Send(string(reqbytes)).
			End()

		if len(errs) > 0 {
			// Just return one of them
			return "", errs[0]
		} else {
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
				return body, nil
			} else if resp.StatusCode == http.StatusNotAcceptable {
				return "", fmt.Errorf("Unable to accept your submission, test161 is out-of-date.  Please update test161 and resubmit.")
			} else {
				return "", fmt.Errorf("The server could not process your request: %v. \nData: %v",
					resp.Status, body)
			}
		}
	}
}

func getRemoteTargetAndValidate(name string) (*test161.TargetListItem, error) {
	var ourVersion *test161.Target
	var serverVersion *test161.TargetListItem
	var ok bool
	ourVersion, ok = env.Targets[name]
	if !ok {
		return nil, fmt.Errorf("Target '%v' does not exist locally. Please update your os161 sources.", name)
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
	submitFlags := flag.NewFlagSet("test161 submit", flag.ExitOnError)
	submitFlags.Usage = usage

	submitFlags.BoolVar(&submitDebug, "debug", false, "")
	submitFlags.BoolVar(&submitVerfiy, "verify", false, "")
	submitFlags.BoolVar(&submitNoCache, "no-cache", false, "")
	submitFlags.Parse(os.Args[2:]) // this may exit

	args := submitFlags.Args()

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
	git, err := gitRepoFromDir(clientConf.SrcDir, submitDebug)
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
		commit, ref, err = git.commitFromTreeish(treeish, submitDebug)
	} else {
		commit, ref, err = git.commitFromHEAD(submitDebug)
	}

	if err != nil {
		return nil, err
	}

	clientConf.git = git
	submitCommit = commit
	submitRef = ref

	return serverVersion, nil
}

// Initialize the cache and key directories in HOME/.test161
func init() {
	if _, err := os.Stat(CACHE_DIR); err != nil {
		if err := os.MkdirAll(CACHE_DIR, 0770); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating cache directory: %v\n", err)
			os.Exit(1)
		}
	}

	if _, err := os.Stat(KEYS_DIR); err != nil {
		if err := os.MkdirAll(KEYS_DIR, 0770); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating keys directory: %v\n", err)
			os.Exit(1)
		}
	}
}

func getKeyHash(user string) string {
	file := path.Join(KEYS_DIR, user, "id_rsa")
	if _, err := os.Stat(file); err != nil {
		return ""
	}

	data, err := ioutil.ReadFile(file)
	if err != nil {
		return ""
	}

	raw := md5.Sum(data)
	hash := strings.ToLower(hex.EncodeToString(raw[:]))

	return hash
}
