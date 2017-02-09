package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/fatih/color"
	"github.com/ops-class/test161"
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

Your submission will receive the following estimated scores:%v

Do you certify that you have followed the collaboration guidelines and wish to submit now?
`

const NoUsersErr = `No users have been configured for test161. Please use 'test161 config add-user' to add users.
(See 'test161 help' for a more detailed command description).
`

// Run the submission locally, but as close to how the server would do it
// as possible
func localSubmitTest(req *test161.SubmissionRequest) (scores []*scoreMapEntry, errs []error) {

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

	scores = splitScores(submission.Tests)

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

	if err := getSubmitArgs(); err != nil {
		printRunError(err)
		return
	}

	// Early sanity checks
	if len(clientConf.Users) == 0 {
		fmt.Fprintf(os.Stderr, NoUsersErr)
		return
	}

	// Check the version of git to figure out if we can even build locally
	if ok, err := checkGitVersionAndComplain(); err != nil {
		err = fmt.Errorf("Unable to check Git version: %v", err)
		printRunError(err)
		return
	} else if !ok {
		return
	}

	// Check that the target exists, both locally and remotely
	if targetInfo, err := getRemoteTargetAndValidate(submitTargetName); err != nil {
		printRunError(err)
		return
	} else {
		collabMsg = targetInfo.CollabMsg
	}

	// Set the Git commit ID and repo info
	git, err := getSubmitCommitIDAndValidate()
	if err != nil {
		printRunError(err)
		return
	}

	// At this point we've checked most of the things we can locally. Before we
	// build, check with the server to make sure this submission is acceptable.

	req := &test161.SubmissionRequest{
		Target:        submitTargetName,
		Users:         clientConf.Users,
		Repository:    git.remoteURL,
		CommitID:      submitCommit,
		CommitRef:     submitRef,
		ClientVersion: test161.Version,
	}

	// Get the current hash of our test161 private key(s). The server will push
	// down new keys if these differ from what it computes.
	for _, user := range req.Users {
		user.KeyHash = getKeyHash(user.Email)
	}

	// Validate before running locally (and install their keys)
	if err := validateSubmission(req); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return
	}

	// Finally, explicitly check test the deployment key. Everything before this point
	// was configured to use either the deployment key or local key. The result of the
	// user validation may have returned a different key (in case of key change), or
	// the initial key. Now, explicitly check the key before the build process to provide
	// a somewhat less cryptic message.
	git.gitSSHCommand = getGitSSHCommand()
	if git.gitSSHCommand == "" {
		fmt.Fprintf(os.Stderr, "Unable to test your deployment key: no deployment keys found")
	}

	if err := git.verifyDeploymentKey(submitDebug); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to verify your deployment key. Please make sure your test161 deployment key is attached to your repository.\n")
		fmt.Fprintf(os.Stderr, "Err: %v\n", err)
		return
	}

	// We've verified what we can at this point, so if only -verify, we're done
	if submitVerfiy {
		exitcode = 0
		fmt.Println("OK")
		return
	}

	// This is a good time to kick off a usage stats upload.
	runTest161Uploader()

	// Local build
	scores, errs := localSubmitTest(req)
	if len(errs) > 0 {
		printRunErrors(errs)
		return
	}

	hasPoints := false
	for _, entry := range scores {
		if entry.Earned > 0 {
			hasPoints = true
			break
		}
	}

	// Don't bother proceeding if no points earned
	if !hasPoints {
		fmt.Println("No points will be earned for this submission, cancelling submission.")
		return
	}

	// Show score and collab policy, and give them a chance to cancel

	scoreMsg := ""
	bold := color.New(color.Bold).SprintFunc()

	for _, entry := range scores {
		name := entry.TargetName
		if entry.IsMeta {
			name = "(" + name + ")"
		}
		temp := fmt.Sprintf("\n%-15v: %v out of %v", name, entry.Earned, entry.Avail)
		scoreMsg += bold(temp)
	}

	fmt.Printf(SubmitMsg, collabMsg, scoreMsg)
	if text := getYesOrNo(); text == "no" {
		fmt.Println()
		fmt.Println("Submission request cancelled")
		fmt.Println()
		return
	}

	// Confirm the users
	for i, u := range req.Users {
		fmt.Printf("\n(%v of %v): You are submitting on behalf of %v. Is this correct?\n",
			i+1, len(req.Users), u.Email)
		if text := getYesOrNo(); text == "no" {
			fmt.Println()
			fmt.Println("Submission request cancelled")
			fmt.Println()
			return
		}
	}

	// Let the server know what we think we're going to get
	req.EstimatedScores = make(map[string]uint)
	for _, entry := range scores {
		req.EstimatedScores[entry.TargetName] = entry.Earned
	}

	// Finally, submit
	if err := submit(req); err == nil {
		fmt.Println("Your submission has been created and is being processed by the test161 server")
		exitcode = 0
	} else {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}

	return
}

// Validate the user and submission info on the server, and update the users'
// private keys that are returned by the server. Fail if the user hasn't set up
// a key yet.
func validateSubmission(req *test161.SubmissionRequest) error {
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
			fmt.Println("Installing deployment key for", data.User)
		} else {
			emptyCount += 1
			fmt.Fprintf(os.Stderr, "Warning: No test161 key exists for %v", data.User)
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

	var pr *PostRequest

	if validateOnly {
		pr = NewPostRequest(ApiEndpointValidate)
	} else {
		pr = NewPostRequest(ApiEndpointSubmit)
	}

	pr.SetType(PostTypeJSON)
	if err := pr.QueueJSON(req, ""); err != nil {
		return "", err
	}

	resp, body, errs := pr.Submit()

	if len(errs) > 0 {
		errs = connectionError(pr.Endpoint, errs)
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

func getSubmitArgs() error {
	submitFlags := flag.NewFlagSet("test161 submit", flag.ExitOnError)
	submitFlags.Usage = usage

	submitFlags.BoolVar(&submitDebug, "debug", false, "")
	submitFlags.BoolVar(&submitVerfiy, "verify", false, "")
	submitFlags.BoolVar(&submitNoCache, "no-cache", false, "")
	submitFlags.Parse(os.Args[2:]) // this may exit

	// Handle positional args
	args := submitFlags.Args()

	if len(args) == 0 {
		return errors.New("test161 submit: Missing target name. run test161 help for detailed usage")
	} else if len(args) > 2 {
		return errors.New("test161 submit: Too many arguments. run test161 help for detailed usage")
	}

	submitTargetName = args[0]
	if len(args) == 2 {
		submitCommit = args[1]
	}

	return nil
}

// Translate the ref passed to a commit ID or get the info from the tip of the
// current branch if nothing was specified.
func getSubmitCommitIDAndValidate() (*gitRepo, error) {

	git, err := gitRepoFromDir(clientConf.SrcDir, submitDebug)
	if err != nil {
		return nil, err
	}

	if !git.canSubmit() {
		// This prints its own message
		return nil, errors.New("Unable to submit")
	}

	commit, ref := "", ""

	if len(submitCommit) > 0 {
		commit, ref, err = git.commitFromTreeish(submitCommit, submitDebug)
	} else {
		commit, ref, err = git.commitFromHEAD(submitDebug)
	}

	if err != nil {
		return nil, err
	}

	submitCommit = commit
	submitRef = ref

	return git, nil
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
