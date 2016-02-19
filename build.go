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

var ksecprintfExp = regexp.MustCompile(`.*ksecprintf\(SECRET, .+, "(.+)"\);.*`)
var successExp = regexp.MustCompile(`.*success\(.+, SECRET, "(.+)"\);.*`)

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
	Repo             string // The git repository to clone
	CommitID         string // The git commit id (HEAD, hash, etc.) to check out
	KConfig          string // The os161 kernel config file for the build
	RequiredCommit   string // A commit required to be in git log
	CacheDir         string // Cache for previous builds
	RequiresUserland bool   // Does userland need to be built?
	Overlay          string // The overlay to use (append to overlay dir in env)
	Users            []string
}

// Use the BuildConf to create a sequence of commands that will build an os161 kernel
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

// Figure out the build directory location, create it if it doesn't exist, and
// lock it.
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

	res := &BuildResults{
		RootDir: t.rootDir,
	}

	if t.isTempDir {
		res.TempDir = t.dir
	}

	return res, nil
}

func commitCheckHandler(t *BuildTest, command *BuildCommand) error {
	for _, l := range command.Output {
		if t.conf.RequiredCommit == l.Line {
			return nil
		}
	}

	return errors.New("Cannot find required commit id")
}

func (t *BuildTest) setCommandEnv() {
	keyfile := ""
	studentDir := ""

	// Get env for git clone/fetch. Pick one of the partners that has a key.
	for _, user := range t.conf.Users {
		studentDir = path.Join(t.env.KeyDir, user)
		temp := path.Join(path.Join(studentDir, "id_rsa"))
		if _, err := os.Stat(temp); err == nil {
			// File exists
			keyfile = temp
			break
		}
	}

	t.cmdEnv = os.Environ()

	if keyfile != "" {
		t.cmdEnv = append(t.cmdEnv, fmt.Sprintf(`GIT_SSH=%v`, path.Join(studentDir, "setssh.sh")))
	}
}

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

		// Try ksecprintf and success (single lines only).
		var res []string
		if res = ksecprintfExp.FindStringSubmatch(line); len(res) == 0 {
			res = successExp.FindStringSubmatch(line)
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
	os.Remove(filename)
	err = os.Rename(outfile, path.Join(t.srcDir, filename))

	done <- err
}

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

	return err

}

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
