package main

import (
	"errors"
	"fmt"
	"github.com/ops-class/test161"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type gitRepo struct {
	dir           string
	remoteName    string
	remoteRef     string
	remoteURL     string
	localRef      string
	remoteUpdated bool
	gitSSHCommand string
}

var minGitVersion = test161.ProgramVersion{
	Major:    2,
	Minor:    3,
	Revision: 0,
}

const GitUpgradeInst = `
Your version of Git must be at least 2.3.0 (you're running %v).

To upgrade on Ubuntu, perform the following commands to add the Git stable ppa, and
install the latest version of Git:

    sudo add-apt-repository ppa:git-core/ppa
    sudo apt-get update
    sudo apt-get install -y git

`
const (
	DoNotUseDeployKey = iota
	UseDeployKeyOnly
	TryDeployKey
)

type gitCmdSpec struct {
	cmdline    string
	allowEmpty bool
	debug      bool
	deployKey  int // Defaults to not use
}

func (git *gitRepo) setRemoteInfo(debug bool) error {

	// Infer the remote name and branch. We can get what we need if they're on a branch
	// and it's set up to track a remote.

	upstreamCmd := &gitCmdSpec{
		cmdline: "git rev-parse --abbrev-ref --symbolic-full-name @{u}",
		debug:   debug,
	}

	if remoteInfo, err := git.doOneCommand(upstreamCmd); err == nil {
		where := strings.Index(remoteInfo, "/")
		if where < 0 {
			// This shouldn't happen, but you never know
			return fmt.Errorf("git rev-parse not of format remote/branch: %v", remoteInfo)
		}
		git.remoteName = remoteInfo[0:where]
		git.remoteRef = remoteInfo[where+1:]

		// Get the URL of the remote
		urlCmd := &gitCmdSpec{
			cmdline: fmt.Sprintf("git ls-remote --get-url %v", git.remoteName),
			debug:   debug,
		}

		if url, err := git.doOneCommand(urlCmd); err != nil {
			fmt.Println(url, err)
			return err
		} else {
			git.remoteURL = url
		}
	} else {
		return err
	}

	return nil
}

const remoteErr = `Your current branch is not set up to track a remote, Use 'git branch -u <upstream>'
to set the upstream for this branch, if one exists. If this is a new branch, use
'git push -u <remote> [<branch>]' to push the new branch to your remote. See
'man git branch' and 'man git push' for more information.
`

const httpErr = `test161 will not accept submissions with http or https repository URLs. Please
use 'git remote set-url <remote_name> <url>' to change your upstream, where <url>
is the SSH URL of your repository (i.e. git@...).
`

func (git *gitRepo) canSubmit() bool {
	if git.remoteURL == "" {
		fmt.Fprintf(os.Stderr, remoteErr)
		return false
	} else if strings.HasPrefix(git.remoteURL, "http") {
		fmt.Fprintf(os.Stderr, httpErr)
		return false
	}
	return true
}

// Get the commit corresponding to HEAD, and check for modifications, remote up-to-date, etc.
func (git *gitRepo) commitFromHEAD(debug bool) (commit, ref string, err error) {

	ref = ""
	commit = ""
	var dirty, ok bool

	// Check for local modifications or untracked files
	if dirty, err = git.isLocalDirty(debug); err != nil {
		err = fmt.Errorf("Cannot determine local status: %v", err)
		return
	} else if dirty {
		err = errors.New("Submission not permitted while changes exist in your working directory\nRun git status to see what files have changed.")
		return
	}

	if git.localRef == "HEAD" {
		fmt.Fprintf(os.Stderr, "Warning: You are in a detached HEAD state, submitting HEAD commit\n")
		ref = "HEAD"
	} else if git.remoteName == "" || git.remoteRef == "" {
		fmt.Fprintf(os.Stderr, "Warning: No remote name or ref, submitting HEAD commit\n")
		ref = "HEAD"
	} else {
		// Try the deploy key, but don't fail if it doesn't exist.
		// We'll explicitly check later when before we build.

		// Check for changes with the remote
		ref = git.remoteName + "/" + git.remoteRef
		if ok, err = git.isRemoteUpToDate(debug, TryDeployKey); err != nil {
			err = fmt.Errorf("Cannot determine remote status: %v", err)
			return
		} else if !ok {
			err = errors.New("Your remote is not up-to-date with your local branch. Please push any changes or specify a commit id.")
			return
		}
	}

	// Finally, get the commit id from the ref
	commitCmd := &gitCmdSpec{
		cmdline: "git rev-parse " + ref,
		debug:   debug,
	}

	if commit, err = git.doOneCommand(commitCmd); err != nil {
		err = fmt.Errorf("Cannot rev-parse ref %v: %v", ref, err)
	}

	return
}

// Get the commit ID from a treeish string, which may be a hex commit id, tag, or branch.
// It's OK if we have modifications, detached head, etc.; we just need to find the commit,
// which we can do if its remote/branch or a tag on the tracked remote.
func (git *gitRepo) commitFromTreeish(treeish string, debug bool) (commit, ref string, err error) {

	commit, ref = "", ""
	var ok bool

	// Break this down into remote/branch for
	where := strings.Index(treeish, "/")
	if where > 0 {
		git.remoteName = treeish[0:where]
		git.remoteRef = treeish[where+1:]
	} else {
		git.remoteRef = treeish
	}

	// First, figure out if this is a ref or a commit id
	if ok, err = regexp.MatchString("^[0-9a-f]+$", treeish); ok {
		// Done, it's just the commit it.
		commit = treeish
	} else {
		// See if we can actually find the ref.
		if ok, err = git.verifyLocalRef(treeish, debug); err != nil {
			err = fmt.Errorf("Error verifying local ref '%v': %v", treeish, err)
			return
		} else if !ok {
			err = fmt.Errorf("Unable to verify local ref '%v'", treeish)
			return
		} else if ok, err = git.verifyRemoteRef(git.remoteRef, debug, TryDeployKey); err != nil {
			err = fmt.Errorf("Error verifying remote ref '%v': %v", treeish, err)
			return
		} else if !ok {
			err = fmt.Errorf("Unable to verify remote ref '%v'", treeish)
			return
		}

		// Get the commit id
		ref = treeish
		commitCmd := &gitCmdSpec{
			cmdline: "git rev-parse " + ref,
			debug:   debug,
		}
		commit, err = git.doOneCommand(commitCmd)
		if err != nil {
			err = fmt.Errorf("Cannot rev-parse ref %v: %v", ref, err)
		}
	}
	return
}

// Infer all of the Git information we can from the source directory. Some of this
// depends on how they set things up and if they are on a branch or detached.
func gitRepoFromDir(src string, debug bool) (*gitRepo, error) {
	git := &gitRepo{}
	git.dir = src

	// Verify that we're in a git repo
	statusCmd := &gitCmdSpec{
		cmdline:    "git status",
		allowEmpty: true,
		debug:      debug,
	}
	if res, err := git.doOneCommand(statusCmd); err != nil {
		return nil, fmt.Errorf("%v", res)
	}

	// This might fail, and if it does, we'll deal with it at submission time.
	if err := git.setRemoteInfo(debug); err != nil && debug {
		return nil, err
	}

	// Get the local branch (or HEAD if detached). We'll need this if submitting without
	// specifying the branch/tag/commit.
	branchCmd := &gitCmdSpec{
		cmdline: "git rev-parse --abbrev-ref HEAD",
		debug:   debug,
	}

	if branch, err := git.doOneCommand(branchCmd); err == nil {
		git.localRef = branch
	}

	// Finally, set the ssh command we'll use for Git
	git.gitSSHCommand = getGitSSHCommand()

	return git, nil
}

func getGitSSHCommand() string {
	users := []string{}
	for _, user := range clientConf.Users {
		users = append(users, user.Email)
	}

	if len(users) > 0 {
		return test161.GetDeployKeySSHCmd(users, KEYS_DIR)
	} else {
		return ""
	}
}

func (git *gitRepo) doOneCommand(gitCmd *gitCmdSpec) (string, error) {
	args := strings.Split(gitCmd.cmdline, " ")
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = git.dir

	if git.gitSSHCommand != "" && gitCmd.deployKey != DoNotUseDeployKey {
		cmd.Env = append(os.Environ(), git.gitSSHCommand)
		if gitCmd.debug {
			fmt.Println("Env:", git.gitSSHCommand)
		}
	}

	if gitCmd.debug {
		fmt.Println(gitCmd.cmdline)
	}

	output, err := cmd.CombinedOutput()

	if gitCmd.debug {
		fmt.Println(string(output))
	}

	// Just trying, but fall back to local authentication for the command
	if err != nil && gitCmd.deployKey == TryDeployKey && git.gitSSHCommand != "" {
		if gitCmd.debug {
			fmt.Println("Git command failed using deployment key:", err)
			fmt.Println("Falling back to local authentication")
		}
		cmdCopy := *(gitCmd)
		cmdCopy.deployKey = DoNotUseDeployKey
		return git.doOneCommand(&cmdCopy)
	} else if err != nil {
		return "", fmt.Errorf(`Failed executing command "%v": %v`, gitCmd.cmdline, err)
	} else if len(output) == 0 && !gitCmd.allowEmpty {
		return "", fmt.Errorf(`No output from "%v"`, gitCmd.cmdline)
	}

	return strings.TrimSpace(string(output)), err
}

func (git *gitRepo) updateRemote(debug bool, deployKey int) error {
	// Update the local refs
	updateCmd := &gitCmdSpec{
		cmdline:    "git remote update " + git.remoteName,
		debug:      debug,
		allowEmpty: true,
		deployKey:  deployKey,
	}
	_, err := git.doOneCommand(updateCmd)
	if err != nil {
		git.remoteUpdated = true
	}
	return err
}

func (git *gitRepo) lookForRef(cmd, ref string, debug bool, deployKey int) (bool, error) {

	gitCmd := &gitCmdSpec{
		cmdline:    cmd,
		debug:      debug,
		allowEmpty: true,
		deployKey:  deployKey,
	}

	res, err := git.doOneCommand(gitCmd)
	if err != nil {
		return false, err
	}

	search := []string{
		"refs/heads/",
		"refs/tags/",
		"refs/remotes/",
	}

	lines := strings.Split(res, "\n")
	for _, line := range lines {
		for _, s := range search {
			if strings.Contains(line, s+ref) {
				return true, nil
			}
		}
	}
	return false, nil
}

// Verfify a ref exists locally. Ref could be a branch head or tag.
func (git *gitRepo) verifyLocalRef(ref string, debug bool) (bool, error) {
	return git.lookForRef("git show-ref", ref, debug, DoNotUseDeployKey)
}

// Verify a ref exists remotely. Ref could be a branch head or tag.
func (git *gitRepo) verifyRemoteRef(ref string, debug bool, deployKey int) (bool, error) {
	return git.lookForRef("git ls-remote "+git.remoteName, ref, debug, deployKey)
}

// Determine if the working directory has uncommitted work
func (git *gitRepo) isLocalDirty(debug bool) (bool, error) {
	// Just check if git status --porcelain outputs anything

	dirtyCmd := &gitCmdSpec{
		cmdline:    "git status --porcelain",
		allowEmpty: true,
		debug:      debug,
	}

	if res, err := git.doOneCommand(dirtyCmd); err != nil {
		return false, err
	} else {
		return len(res) > 0, nil
	}
}

// Determine if the remote is up-to-date with the local.
func (git *gitRepo) isRemoteUpToDate(debug bool, deployKey int) (bool, error) {

	if git.remoteName == "" {
		return false, errors.New("Cannot determine if your remote is up-to-date, undetermined remote name")
	}

	if !git.remoteUpdated {
		if err := git.updateRemote(debug, deployKey); err != nil {
			return false, err
		}
	}

	// Get our local commit
	gitCmd := &gitCmdSpec{
		cmdline: "git rev-parse HEAD",
		debug:   debug,
	}
	localCommit, err := git.doOneCommand(gitCmd)
	if err != nil {
		return false, err
	}

	// Get the remote commit
	gitCmd = &gitCmdSpec{
		cmdline: fmt.Sprintf("git rev-parse %v/%v", git.remoteName, git.remoteRef),
		debug:   debug,
	}

	remoteCommit, err := git.doOneCommand(gitCmd)
	if err != nil {
		return false, err
	}

	return localCommit == remoteCommit, nil
}

// Normally, this is 'git version M.m.r', but we've seen -rcN tacked on for
// release candidates.
var gitVersionRegexp *regexp.Regexp = regexp.MustCompile(`^git version (\d+)\.(\d+)\.(\d+).*$`)

func gitVersion() (ver test161.ProgramVersion, err error) {

	var verText string

	git := &gitRepo{}
	if verText, err = git.doOneCommand(&gitCmdSpec{cmdline: "git version"}); err != nil {
		return
	}

	if res := gitVersionRegexp.FindStringSubmatch(verText); len(res) == 4 {
		maj, _ := strconv.Atoi(res[1])
		min, _ := strconv.Atoi(res[2])
		rev, _ := strconv.Atoi(res[3])
		ver.Major = uint(maj)
		ver.Minor = uint(min)
		ver.Revision = uint(rev)
	} else {
		err = fmt.Errorf("`git version` does not match expected output: %v", verText)
	}

	return
}

// Compare the current version of git vs. our required version. Return true
// if the current version meets our requirement, false otherwise. If the verison
// is not recent enough, tell the user how to upgrade.
func checkGitVersionAndComplain() (bool, error) {
	ver, err := gitVersion()
	if err != nil {
		return false, err
	}

	// At least min version
	if ver.CompareTo(minGitVersion) >= 0 {
		return true, nil
	} else {
		fmt.Printf(GitUpgradeInst, ver)
		return false, nil
	}
}

func (git *gitRepo) verifyDeploymentKey(debug bool) error {
	git.remoteUpdated = false //force
	return git.updateRemote(debug, UseDeployKeyOnly)
}
