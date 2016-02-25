package main

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

type gitRepo struct {
	dir           string
	remoteName    string
	remoteRef     string
	remoteURL     string
	localRef      string
	remoteUpdated bool
}

func (git *gitRepo) setRemoteInfo(debug bool) error {

	// Infer the remote name and branch. We can get what we need if they're on a branch
	// and it's set up to track a remote.
	if remoteInfo, err := doOneCommand("git rev-parse --abbrev-ref --symbolic-full-name @{u}", git.dir, false, debug); err == nil {
		where := strings.Index(remoteInfo, "/")
		if where < 0 {
			// This shouldn't happen, but you never know
			return fmt.Errorf("git rev-parse not of format remote/branch: %v", remoteInfo)
		}
		git.remoteName = remoteInfo[0:where]
		git.remoteRef = remoteInfo[where+1:]

		// Get the URL of the remote
		cmd := fmt.Sprintf("git ls-remote --get-url %v", git.remoteName)
		if url, err := doOneCommand(cmd, git.dir, false, debug); err != nil {
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

const remoteErr = `
Your current branch is not set up to track a remote, and there is no repository specified
in your .test161.conf file. Use 'git branch -u' to set the upstream for this branch.
`

const remoteWarning = `
WARNING: Your current branch is not set up to track a remote Use 'git branch -u' to set
the upstream for this branch. Submit will use the repository URL found in your .test161.conf
file
`

func (git *gitRepo) canSubmit() bool {
	if git.remoteURL == "" && clientConf.Repository == "" {
		fmt.Println(remoteErr)
		return false
	} else if git.remoteURL == "" {
		git.remoteURL = clientConf.Repository
		fmt.Println(remoteWarning)
		// OK, but not advised
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
		err = errors.New("Submission not permitted while changes exist in your working directory")
		return
	}

	if git.localRef == "HEAD" {
		fmt.Println("Warning: your are in a detached HEAD state, submitting HEAD commit")
		ref = "HEAD"
	} else if git.remoteName == "" || git.remoteRef == "" {
		fmt.Println("Warning: No remote name or ref, submitting HEAD commit")
		ref = "HEAD"
	} else {
		// Check for changes with the remote
		ref = git.remoteName + "/" + git.remoteRef
		if ok, err = git.isRemoteUpToDate(debug); err != nil {
			err = fmt.Errorf("Cannot determine remote status: %v", err)
			return
		} else if !ok {
			err = errors.New("Your remote is not up-to-date with your local branch. Please push any changes or specify a commit id.")
			return
		}
	}

	// Finally, get the commit id from the ref
	if commit, err = doOneCommand("git rev-parse "+ref, git.dir, false, debug); err != nil {
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
		} else if ok, err = git.verifyRemoteRef(git.remoteRef, debug); err != nil {
			err = fmt.Errorf("Error verifying remote ref '%v': %v", treeish, err)
			return
		} else if !ok {
			err = fmt.Errorf("Unable to verify remote ref '%v'", treeish)
			return
		}

		// Get the commit id
		ref = treeish
		commit, err = doOneCommand("git rev-parse "+ref, git.dir, false, debug)
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
	if res, err := doOneCommand("git status", src, true, false); err != nil {
		return nil, fmt.Errorf("%v", res)
	}

	// This might fail, and if it does, we'll deal with it at submission time.
	if err := git.setRemoteInfo(debug); err != nil && debug {
		return nil, err
	}

	// Get the local branch (or HEAD if detached). We'll need this if submitting without
	// specifying the branch/tag/commit.
	if branch, err := doOneCommand("git rev-parse --abbrev-ref HEAD", src, false, debug); err == nil {
		git.localRef = branch
	}

	return git, nil
}

func doOneCommand(cmdline, srcDir string, allowEmpty, verbose bool) (string, error) {
	args := strings.Split(cmdline, " ")
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = srcDir

	if verbose {
		fmt.Println(cmdline)
	}

	output, err := cmd.CombinedOutput()

	if verbose {
		fmt.Println(string(output))
	}

	if err != nil {
		return "", fmt.Errorf(`Cannot execute command "%v": %v`, cmdline, err)
	} else if len(output) == 0 && !allowEmpty {
		return "", fmt.Errorf(`No output from "%v"`, cmdline)
	}

	return strings.TrimSpace(string(output)), err
}

func (git *gitRepo) updateRemote(debug bool) error {
	// Update the local refs
	_, err := doOneCommand("git remote update "+git.remoteName, git.dir, true, debug)
	if err != nil {
		git.remoteUpdated = true
	}
	return err
}

func (git *gitRepo) lookForRef(cmd, ref string, debug bool) (bool, error) {
	res, err := doOneCommand(cmd, git.dir, true, debug)
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
	return git.lookForRef("git show-ref", ref, debug)
}

// Verify a ref exists remotely. Ref could be a branch head or tag.
func (git *gitRepo) verifyRemoteRef(ref string, debug bool) (bool, error) {
	return git.lookForRef("git ls-remote "+git.remoteName, ref, debug)
}

// Determine if the working directory has uncommitted work
func (git *gitRepo) isLocalDirty(debug bool) (bool, error) {
	// Just check if git status --porcelain outputs anything
	if res, err := doOneCommand("git status --porcelain", git.dir, true, debug); err != nil {
		return false, err
	} else {
		return len(res) > 0, nil
	}
}

// Determine if the remote is up-to-date with the local.
func (git *gitRepo) isRemoteUpToDate(debug bool) (bool, error) {

	if git.remoteName == "" {
		return false, errors.New("Cannot determine if your remote is up-to-date, undetermined remote name")
	}

	if !git.remoteUpdated {
		if err := git.updateRemote(debug); err != nil {
			return false, err
		}
	}

	// Get our local commit
	localCommit, err := doOneCommand("git rev-parse HEAD", git.dir, false, debug)
	if err != nil {
		return false, err
	}

	// Get the remote commit
	cmdLine := fmt.Sprintf("git rev-parse %v/%v", git.remoteName, git.remoteRef)
	remoteCommit, err := doOneCommand(cmdLine, git.dir, false, debug)
	if err != nil {
		return false, err
	}

	return localCommit == remoteCommit, nil
}
