package test161

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/satori/go.uuid"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"sync"
)

// Regular expressions for secure output. Only lines that match these expressions in
// files that we trust will have SECRET replaced with the actual key.
var (
	secprintfExp = regexp.MustCompile(`.*secprintf\(SECRET, .+, "(.+)"\);.*`)
	successExp   = regexp.MustCompile(`.*success\(.+, SECRET, "(.+)"\);.*`)
	partialExp   = regexp.MustCompile(`.*partial_credit\(SECRET, "(.+)",.+\);.*`)
)

func GetDeployKeySSHCmd(users []string, keyDir string) string {

	// We try both keys in case only one is setup
	keyfiles := []string{}

	for _, user := range users {
		studentDir := path.Join(keyDir, user)
		temp := path.Join(path.Join(studentDir, "id_rsa"))
		if _, err := os.Stat(temp); err == nil {
			// File exists
			keyfiles = append(keyfiles, temp)
		}
	}

	if len(keyfiles) > 0 {
		cmd := "GIT_SSH_COMMAND=ssh -o StrictHostKeyChecking=no -o IdentitiesOnly=yes"
		for _, key := range keyfiles {
			cmd += fmt.Sprintf(" -i %v", key)
		}
		return cmd
	} else {
		return ""
	}

}

// BuildTest is a variant of a Test, and specifies how the build process should work.
// We obey the same schema so the front end tools can treat this like any other test.
type BuildTest struct {

	// Mongo ID
	ID string `yaml:"-" json:"id" bson:"_id,omitempty"`

	// Metadata
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`

	Commands []*BuildCommand `json:"commands"` // Protected by L
	Status   []Status        `json:"status"`   // Protected by L
	Result   TestResult      `json:"result"`   // Protected by L

	// Dependency data
	DependencyID string `json:"depid"`
	IsDependency bool   `json:"isdependency"`

	// Grading.  These are set when the test is being run as part of a Target.
	PointsAvailable uint   `json:"points_avail" bson:"points_avail"`
	PointsEarned    uint   `json:"points_earned" bson:"points_earned"`
	ScoringMethod   string `json:"scoring_method" bson:"scoring_method"`

	startTime TimeFixedPoint
	dir       string // The base (temp) directory for the build.
	wasCached bool   // Was the base directory cached
	isTempDir bool   // Is the build directory a temp dir that should be removed?
	srcDir    string // Directory for the os161 source code (dir/src)
	rootDir   string // Directory for compilation output (dir/root)

	users []string

	conf    *BuildConf
	env     *TestEnvironment
	cmdEnv  []string
	keyLock sync.Mutex
}

// A variant of a Test Command for builds
type BuildCommand struct {
	ID string `yaml:"-" json:"id" bson:"_id,omitempty"`

	Type  string    `json:"type"`
	Input InputLine `json:"input"`

	// Set during target init
	PointsAvailable uint `json:"points_avail" bson:"points_avail"`
	PointsEarned    uint `json:"points_earned" bson:"points_earned"`

	// Set during testing
	Output []*OutputLine `json:"output"`

	// Set during evaluation
	Status string `json:"status"`

	test     *BuildTest
	startDir string                                // The directory to run this command in
	handler  func(*BuildTest, *BuildCommand) error // Invoke after command exits to determine success
}

// BuildConf specifies the configuration for building os161.
type BuildConf struct {
	Repo             string   // The git repository to clone
	CommitID         string   // The git commit id (HEAD, hash, etc.) to check out
	KConfig          string   // The os161 kernel config file for the build
	RequiredCommit   string   // A commit required to be in git log
	CacheDir         string   // Cache for previous builds
	RequiresUserland bool     // Does userland need to be built?
	Overlay          string   // The overlay to use (append to overlay dir in env)
	Users            []string // The users who own the repo. Needed for the finding the key.
}

// Use the BuildConf to create a sequence of commands that will build an os161 kernel
// and userspace binaries (ASST2+).
func (b *BuildConf) ToBuildTest(env *TestEnvironment) (*BuildTest, error) {

	t := &BuildTest{
		ID:              uuid.NewV4().String(),
		Name:            "build",
		Description:     "Clone Git repository and build kernel",
		Commands:        make([]*BuildCommand, 0),
		Result:          TEST_RESULT_NONE,
		DependencyID:    "build",
		IsDependency:    true,
		PointsAvailable: uint(0),
		PointsEarned:    uint(0),
		ScoringMethod:   TEST_SCORING_ENTIRE,
		conf:            b,
		env:             env,
	}

	if err := t.initDirs(); err != nil {
		return nil, err
	}

	t.addGitCommands()
	t.addOverlayCommand()
	t.addBuildCommands()

	return t, nil
}

// Get the root directory of the build output
func (t *BuildTest) RootDir() string {
	return t.rootDir
}

// Convert a command's raw output into individual OutputLines
func makeLines(rawoutput []byte) []*OutputLine {
	lines := strings.Split(string(rawoutput), "\n")
	output := make([]*OutputLine, len(lines))

	for i, l := range lines {
		output[i] = &OutputLine{
			Line:     l,
			SimTime:  TimeFixedPoint(i),
			WallTime: TimeFixedPoint(i),
		}
	}
	return output
}

// Execute an individual BuildTest command
func (cmd *BuildCommand) Run(env *TestEnvironment) error {
	tokens := strings.Split(cmd.Input.Line, " ")
	if len(tokens) < 1 {
		return errors.New("BuildCommand: Empty command")
	}

	cmd.Output = make([]*OutputLine, 0)

	// Add a line indicating what the build process is doing
	cmd.Output = append(cmd.Output, &OutputLine{
		Line:     "Exec: " + cmd.Input.Line,
		SimTime:  TimeFixedPoint(1),
		WallTime: TimeFixedPoint(1),
	})

	cmd.Status = COMMAND_STATUS_RUNNING

	env.notifyAndLogErr("Build Command Status", cmd,
		MSG_PERSIST_UPDATE, MSG_FIELD_OUTPUT|MSG_FIELD_STATUS)

	c := exec.Command(tokens[0], tokens[1:]...)
	c.Dir = cmd.startDir
	c.Env = cmd.test.cmdEnv

	output, err := c.CombinedOutput()

	if err != nil {
		cmd.Output = append(cmd.Output, makeLines(output)...)
	} else {
		if cmd.handler != nil {
			cmd.Output = makeLines(output)
			err = cmd.handler(cmd.test, cmd)
			if err == nil {
				// Clean up output
				cmd.Output = cmd.Output[0:1]
			}
		}

		// Success
		cmd.Output = append(cmd.Output, &OutputLine{
			Line:     "OK",
			SimTime:  TimeFixedPoint(2),
			WallTime: TimeFixedPoint(2),
		})
	}

	env.notifyAndLogErr("Build Command Output", cmd, MSG_PERSIST_UPDATE, MSG_FIELD_OUTPUT)

	return err
}

type BuildResults struct {
	RootDir string
	TempDir string
}

// Figure out the build directory location, create it if it doesn't exist.
func (t *BuildTest) initDirs() (err error) {

	buildDir := ""

	// Try the cache directory first
	if len(t.conf.CacheDir) > 0 {
		if _, err = os.Stat(t.conf.CacheDir); err == nil {
			hashbytes := sha256.Sum256([]byte(t.conf.Repo))
			hash := strings.ToLower(hex.EncodeToString(hashbytes[:]))
			buildDir = path.Join(t.conf.CacheDir, hash)
			if _, err = os.Stat(buildDir); err != nil {
				if err = os.Mkdir(buildDir, 0770); err != nil {
					return
				}
			}
		}
	}

	t.isTempDir = false
	if len(buildDir) == 0 {
		// Use a temp directory instead
		if buildDir, err = ioutil.TempDir("", "os161"); err != nil {
			return
		}
		t.isTempDir = true
	}

	t.dir = buildDir
	t.srcDir = path.Join(buildDir, "src")
	t.rootDir = path.Join(buildDir, "root")

	// TODO: Lock the build directory

	return
}

// Run builds the OS/161 kernel and userspace binaries
func (t *BuildTest) Run(env *TestEnvironment) (*BuildResults, error) {
	var err error

	t.env = env
	t.env.keyMap = make(map[string]string)
	t.setCommandEnv()

	t.Result = TEST_RESULT_RUNNING
	t.env.notifyAndLogErr("Build Test Running", t, MSG_PERSIST_UPDATE, MSG_FIELD_STATUS)

	defer func() {
		env.notifyAndLogErr("Build Test Complete", t, MSG_PERSIST_COMPLETE, 0)
	}()

	for _, c := range t.Commands {

		err = c.Run(env)

		if err != nil {
			c.Status = COMMAND_STATUS_INCORRECT
			t.Result = TEST_RESULT_INCORRECT
		} else {
			c.Status = COMMAND_STATUS_CORRECT
		}

		env.notifyAndLogErr("Build Test Output", c, MSG_PERSIST_UPDATE, MSG_FIELD_STATUS|MSG_FIELD_OUTPUT)

		if err != nil {
			if t.isTempDir {
				os.RemoveAll(t.dir)
			}
			return nil, err
		}
	}

	t.Result = TEST_RESULT_CORRECT

	// Package up the results for the caller
	res := &BuildResults{
		RootDir: t.rootDir,
	}
	if t.isTempDir {
		res.TempDir = t.dir
	}

	return res, nil
}

// Handler function for finding a required commit.
func commitCheckHandler(t *BuildTest, command *BuildCommand) error {
	for _, l := range command.Output {
		if t.conf.RequiredCommit == l.Line {
			return nil
		}
	}

	return errors.New("Cannot find required commit id")
}

// Set up the command environment. Specifically, we need to set the GIT_SSH_COMMAND
// env variable based users' repo we're building. This forces git to use a specific
// key file, which we need because each user generates a deployment key for test161.
func (t *BuildTest) setCommandEnv() {
	t.cmdEnv = os.Environ()
	if cmd := GetDeployKeySSHCmd(t.conf.Users, t.env.KeyDir); cmd != "" {
		t.cmdEnv = append(t.cmdEnv, cmd)
	} else {
		t.env.Log.Println("Missing deployment key for", t.conf.Users)
	}
}

// Add all the Git commands needed to update the repo and checkout the right
// commit.
func (t *BuildTest) addGitCommands() {

	// If we have the repo cached, fetch instead of clone.
	if _, err := os.Stat(t.srcDir); err == nil {
		// First, reset it so we remove previous overlay changes
		t.addCommand("git reset --hard", t.srcDir)
		t.addCommand("git fetch", t.srcDir)
		t.wasCached = true
	} else {
		t.addCommand(fmt.Sprintf("git clone %v src", t.conf.Repo), t.dir)
	}

	t.addCommand(fmt.Sprintf("git checkout %v", t.conf.CommitID), t.srcDir)

	// Before building, we may need to check for a specific commit
	if len(t.conf.RequiredCommit) > 0 {
		cmd := t.addCommand(fmt.Sprintf("git log --pretty=format:%v", "%H"), t.srcDir)
		cmd.handler = commitCheckHandler
	}
}

const KEYBYTES = 32

// Generate a new key for test161 secure output
func newKey(numbytes int) (string, error) {
	bytes := make([]byte, numbytes)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	key := strings.ToLower(hex.EncodeToString(bytes))
	return key, nil
}

// Find the key for id.  If it doesn't exist, create it and add
// it to the environment's keyMap.
func (t *BuildTest) getKey(id string) (key string, err error) {
	t.keyLock.Lock()
	defer t.keyLock.Unlock()

	var ok bool

	if key, ok = t.env.keyMap[id]; ok {
		return key, nil
	} else if key, err = newKey(KEYBYTES); err != nil {
		return "", err
	} else {
		t.env.keyMap[id] = key
		return key, nil
	}
}

// Process a single file in the list of SECURE files in the overlay.
// We replace all instances if SECRET with the per-command key.
func (t *BuildTest) doSecureOverlayFile(filename string, done chan error) {

	var file, out *os.File
	var err error
	var key string

	if len(strings.TrimSpace(filename)) == 0 {
		done <- nil
		return
	}

	if file, err = os.Open(path.Join(t.srcDir, filename)); err != nil {
		done <- err
		return
	}

	// Writes go to a temp file
	outfile := path.Join(t.srcDir, filename+".tmp")
	if out, err = os.Create(outfile); err != nil {
		done <- err
		return
	}

	// NewScanner splits lines by default, but doesn't keep "\n"
	scanner := bufio.NewScanner(file)
	writer := bufio.NewWriter(out)
	defer file.Close()
	defer out.Close()

	for scanner.Scan() {
		line := scanner.Text()

		// Try secprintf and success (single lines only).
		var res []string
		if res = secprintfExp.FindStringSubmatch(line); len(res) == 0 {
			if res = successExp.FindStringSubmatch(line); len(res) == 0 {
				res = partialExp.FindStringSubmatch(line)
			}
		}

		// Get the key and replace SECRET.
		// (res[0] is the full line, res[1] is the command name)
		if len(res) == 2 {
			if key, err = t.getKey(res[1]); err != nil {
				done <- err
				return
			}
			line = strings.Replace(line, "SECRET", fmt.Sprintf(`"%v"`, key), 1)
		}

		// Write the possibly modified line to the temp file
		if _, err = writer.WriteString(line + "\n"); err != nil {
			done <- err
		}
	}
	writer.Flush()

	// Check of the reads failed
	if err = scanner.Err(); err != nil {
		done <- err
	}

	// Finally, rename the temp file
	file.Close()
	out.Close()
	os.Remove(path.Join(t.srcDir, filename))
	err = os.Rename(outfile, path.Join(t.srcDir, filename))

	done <- err
}

// This gets called when the rsync overlay command is executed. This function
// is in charge of substituting SECRET with a per-test key for secure output
// testing.
func overlayHandler(t *BuildTest, command *BuildCommand) error {

	// Read the SECRET file to figure out what we need to overwrite.
	data, err := ioutil.ReadFile(path.Join(t.srcDir, "SECRET"))
	if err != nil {
		return err
	}

	files := strings.Split(string(data), "\n")
	done := make(chan error)
	expected := 0

	// Substitute all of the instances of SECRET with a private key,
	// one for each command.
	for _, f := range files {
		// Last line may or may not have a new line character
		if len(strings.TrimSpace(f)) > 0 {
			expected += 1
			go t.doSecureOverlayFile(f, done)
		}
	}

	// Wait for everyone to finish.
	for i := 0; i < expected; i++ {
		temp := <-done
		if temp != nil {
			// Just take the last error
			err = temp
		}
	}

	if err != nil {
		return err
	}

	// Remove everything from the REMOVED file, if we have one
	if _, err = os.Stat(path.Join(t.srcDir, "REMOVED")); err != nil {
		return nil
	}

	data, err = ioutil.ReadFile(path.Join(t.srcDir, "REMOVED"))
	if err != nil {
		return err
	}

	files = strings.Split(string(data), "\n")

	for _, f := range files {
		// Last line may or may not have a new line character
		if len(strings.TrimSpace(f)) > 0 {
			err = os.RemoveAll(path.Join(t.srcDir, f))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Add the commands required when there is an overlay present. This should always
// happen on the server, but rarely for clients (except testing).
func (t *BuildTest) addOverlayCommand() {
	if len(t.conf.Overlay) == 0 {
		t.env.Log.Println("Warning: no overlay in build configuration")
		return
	}

	overlayPath := path.Join(t.env.OverlayRoot, t.conf.Overlay)
	if _, err := os.Stat(overlayPath); err != nil {
		// It doesn't exist, which is the expected behavior for students' local builds.
		t.env.Log.Println("Skipping overlay (OK for local builds)")
		return
	}

	t.addCommand(fmt.Sprintf("rsync -r %v/ %v", overlayPath, t.srcDir), t.srcDir)
	cmd := t.addCommand("sync", t.srcDir)
	cmd.handler = overlayHandler
}

// Add the chunk of commands needed to build OS/161
func (t *BuildTest) addBuildCommands() error {
	confDir := path.Join(t.srcDir, "kern/conf")
	compDir := path.Join(path.Join(t.srcDir, "kern/compile"), t.conf.KConfig)

	os.RemoveAll(compDir)

	t.addCommand("./configure --ostree="+t.rootDir, t.srcDir)

	if t.conf.RequiresUserland {
		t.addCommand("bmake clean", t.srcDir)
		t.addCommand("bmake", t.srcDir)
		t.addCommand("bmake install", t.srcDir)
	}

	t.addCommand("./config "+t.conf.KConfig, confDir)
	t.addCommand("bmake clean", compDir)
	t.addCommand("bmake", compDir)
	t.addCommand("bmake depend", compDir)
	t.addCommand("bmake install", compDir)

	return nil
}

// Add an individual build command by specifying the command line and
// directory to run from.
func (t *BuildTest) addCommand(cmdLine string, dir string) *BuildCommand {
	cmd := &BuildCommand{
		Type:     "build",
		Output:   []*OutputLine{},
		Status:   COMMAND_STATUS_NONE,
		startDir: dir,
		handler:  nil,
		ID:       uuid.NewV4().String(),
	}
	cmd.Input.Line = cmdLine
	cmd.startDir = dir
	cmd.handler = nil
	cmd.test = t

	t.Commands = append(t.Commands, cmd)

	return cmd
}
