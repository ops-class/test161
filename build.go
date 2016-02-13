package test161

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"gopkg.in/mgo.v2/bson"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
)

// BuildTest is a variant of a Test, and specifies how the build process should work.
// We obey the same schema so the front end tools can treat this like any other test.
type BuildTest struct {

	// Mongo ID
	ID bson.ObjectId `yaml:"-" json:"id" bson:"_id,omitempty"`

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
	wasCached bool   // Was the based directory cached
	srcDir    string // Directory for the os161 source code (dir/src)
	rootDir   string // Directory for compilation output (dir/root)

	conf *BuildConf

	updateChan chan *TestUpdateMsg // Nonblocking write, may be nil
}

// A variant of a Test Command for builds
type BuildCommand struct {
	ID bson.ObjectId `yaml:"-" json:"id" bson:"_id,omitempty"`

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
}

// Use the BuildConf to create a sequence of commands that will build an os161 kernel
func (b *BuildConf) ToBuildTest() (*BuildTest, error) {

	t := &BuildTest{
		ID:              bson.NewObjectId(),
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
	}

	if err := t.initDirs(); err != nil {
		return nil, err
	}

	t.addGitCommands()
	t.addOverlayCommands()
	t.addBuildCommands()

	return t, nil
}

// Get the root directory of the build output
func (t *BuildTest) RootDir() string {
	return t.rootDir
}

func (cmd *BuildCommand) Run() error {
	tokens := strings.Split(cmd.Input.Line, " ")
	if len(tokens) < 1 {
		return errors.New("BuildCommand: Empty command")
	}

	c := exec.Command(tokens[0], tokens[1:]...)
	c.Dir = cmd.startDir

	output, err := c.CombinedOutput()
	if err != nil {
		return err
	}

	lines := strings.Split(string(output), "\n")
	cmd.Output = make([]*OutputLine, len(lines))
	for i, l := range lines {
		cmd.Output[i] = &OutputLine{
			Line:     l,
			SimTime:  TimeFixedPoint(i),
			WallTime: TimeFixedPoint(i),
		}
	}

	return nil
}

type BuildResults struct {
	RootDir string
	KeyMap  map[string]string
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

	// Use a temp directory instead
	if len(buildDir) == 0 {
		if buildDir, err = ioutil.TempDir("", "os161"); err != nil {
			return
		}
	}

	t.dir = buildDir
	t.srcDir = path.Join(buildDir, "src")
	t.rootDir = path.Join(buildDir, "root")

	// TODO: Lock the build directory

	return
}

func (t *BuildTest) Run() (*BuildResults, error) {
	var err error

	t.Result = TEST_RESULT_RUNNING

	for _, c := range t.Commands {
		c.Status = COMMAND_STATUS_RUNNING

		if err = c.Run(); err != nil {
			c.Status = COMMAND_STATUS_INCORRECT
			t.Result = TEST_RESULT_INCORRECT
			return nil, err
		} else {
			if c.handler != nil {
				err = c.handler(t, c)
				if err != nil {
					c.Status = COMMAND_STATUS_INCORRECT
					t.Result = TEST_RESULT_INCORRECT
					return nil, err
				}
			}
			c.Status = COMMAND_STATUS_CORRECT
			// TODO: Broadcast
		}
	}

	t.Result = TEST_RESULT_CORRECT

	res := &BuildResults{
		RootDir: t.rootDir,
		KeyMap:  nil,
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

func (t *BuildTest) addGitCommands() {

	// If we have the repo cached, try a simple checkout.
	if _, err := os.Stat(t.srcDir); err == nil {
		t.addCommand(fmt.Sprintf("git checkout -f %v", t.conf.CommitID), t.srcDir)
		t.wasCached = true
	} else {
		t.addCommand(fmt.Sprintf("git clone %v src", t.conf.Repo), t.dir)
		t.addCommand(fmt.Sprintf("git checkout %v", t.conf.CommitID), t.srcDir)
	}

	// Before building, we may need to check for a specific commit
	if len(t.conf.RequiredCommit) > 0 {
		cmd := t.addCommand(fmt.Sprintf("git log --pretty=format:%v", "%H"), t.srcDir)
		cmd.handler = commitCheckHandler
	}
}

func (t *BuildTest) addOverlayCommands() {

}

func (t *BuildTest) addBuildCommands() error {
	confDir := path.Join(t.srcDir, "kern/conf")
	compDir := path.Join(path.Join(t.srcDir, "kern/compile"), t.conf.KConfig)

	t.addCommand("./configure --ostree="+t.rootDir, t.srcDir)

	if t.conf.RequiresUserland {
		t.addCommand("bmake", t.srcDir)
		t.addCommand("bmake install", t.srcDir)
	}
	t.addCommand("./config "+t.conf.KConfig, confDir)
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
		ID:       bson.NewObjectId(),
	}
	cmd.Input.Line = cmdLine
	cmd.startDir = dir
	cmd.handler = nil
	cmd.test = t

	t.Commands = append(t.Commands, cmd)

	return cmd
}
