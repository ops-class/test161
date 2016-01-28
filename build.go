package test161

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
)

// BuildConf specifies the configuration for building os161.
type BuildConf struct {
	Repo     string // The git repository to clone
	CommitID string // The git commit id (HEAD, hash, etc.) to check out
	Config   string // The os161 kernel config file for the build

	dir     string // The base (temp) directory for the build.
	srcDir  string // Directory for the os161 source code (dir/src)
	rootDir string // Directory for compilation output (dir/root)
}

func NewBuildConf(repo, commit, config string) (*BuildConf, error) {
	tdir, err := ioutil.TempDir("", "os161")
	if err != nil {
		return nil, err
	}

	return &BuildConf{
		dir:      tdir,
		srcDir:   path.Join(tdir, "src"),
		rootDir:  path.Join(tdir, "root"),
		Repo:     repo,
		CommitID: commit,
		Config:   config,
	}, nil
}

// Get the root directory of the build output
func (b *BuildConf) RootDir() string {
	return b.rootDir
}

// CleanUp removes any temp resources created during the build.  Only call this
// when completely done with the compilation output.
func (b *BuildConf) CleanUp() {
	os.RemoveAll(b.dir)
}

func (b *BuildConf) getSources() (string, error) {

	cmds := []*buildCommand{
		&buildCommand{fmt.Sprintf("git clone %v src", b.Repo), b.dir},
		&buildCommand{fmt.Sprintf("git reset --hard %v", b.CommitID), b.srcDir},
	}

	return commandBatch(cmds)
}

// Build OS161.  This assumes the sources have been pulled.
func (b *BuildConf) buildOS161() (string, error) {

	confDir := path.Join(b.srcDir, "kern/conf")
	compDir := path.Join(path.Join(b.srcDir, "kern/compile"), b.Config)

	cmds := []*buildCommand{
		&buildCommand{"./configure --ostree=" + b.rootDir, b.srcDir},
		&buildCommand{"bmake", b.srcDir},
		&buildCommand{"bmake install", b.srcDir},
		&buildCommand{"./config " + b.Config, confDir},
		&buildCommand{"bmake", compDir},
		&buildCommand{"bmake depend", compDir},
		&buildCommand{"bmake install", compDir},
	}

	return commandBatch(cmds)
}

func (b *BuildConf) GitAndBuild() (string, error) {
	o, e := b.getSources()
	if e != nil {
		return o, e
	}

	return b.buildOS161()
}

type buildCommand struct {
	command string
	dir     string
}

func singleCommand(cmd *buildCommand) (string, error) {
	tokens := strings.Split(cmd.command, " ")
	if len(tokens) < 1 {
		return "", errors.New("buildConf: Empty command")
	}

	c := exec.Command(tokens[0], tokens[1:]...)
	c.Dir = cmd.dir
	output, err := c.CombinedOutput()
	return string(output), err
}

func commandBatch(cmds []*buildCommand) (string, error) {
	allOutput := ""

	for _, cmd := range cmds {
		output, err := singleCommand(cmd)
		allOutput += output + "\n"
		if err != nil {
			return allOutput, err
		}
	}

	return allOutput, nil
}
