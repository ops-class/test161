/*
Package test161 implements a library for testing OS/161 kernels. We use expect
to drive the sys161 system simulator and collect useful output using the stat
socket.
*/
package test161

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/kr/pty"
	"github.com/ops-class/test161/expect"
	"github.com/termie/go-shutil"
	// "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Test struct {

	// Mongo ID
	ID bson.ObjectId `yaml:"-" json:"id" bson:"_id,omitempty"`

	// Input

	// Metadata
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Tags        []string `yaml:"tags" json:"tags"`
	Depends     []string `yaml:"depends" json:"depends"`

	// Configuration chunks
	Sys161      Sys161Conf    `yaml:"sys161" json:"sys161"`
	Stat        StatConf      `yaml:"stat" json:"stat"`
	Monitor     MonitorConf   `yaml:"monitor" json:"monitor"`
	CommandConf []CommandConf `yaml:"commandconf" json:"commandconf"`
	Misc        MiscConf      `yaml:"misc" json:"misc"`

	// Actual test commands to run
	Content string `fm:"content" yaml:"-" json:"-" bson:"-"`

	// Big lock that protects most fields shared between Run and getStats
	L *sync.Mutex `json:"-" bson:"-"`

	// Output

	ConfString string         `json:"confstring"` // Only set during once
	WallTime   TimeFixedPoint `json:"walltime"`   // Protected by L
	SimTime    TimeFixedPoint `json:"simtime"`    // Protected by L
	Commands   []*Command     `json:"commands"`   // Protected by L
	Status     []Status       `json:"status"`     // Protected by L
	Result     TestResult     `json:"result"`     // Protected by L

	// Dependency data
	DependencyID string           `json:"depid"`
	ExpandedDeps map[string]*Test `json:"-" bson:"-"`
	IsDependency bool             `json:"isdependency"`

	// Grading.  These are set when the test is being run as part of a Target.
	PointsAvailable uint   `json:"points_avail" bson:"points_avail"`
	PointsEarned    uint   `json:"points_earned" bson:"points_earned"`
	ScoringMethod   string `json:"scoring_method" bson:"scoring_method"`

	// Unproctected Private fields
	tempDir     string           // Only set once
	startTime   int64            // Only set once
	statStarted bool             // Only changed once
	env         *TestEnvironment // Set at top of Run
	allCorrect  bool

	sys161         *expect.Expect // Protected by L
	running        bool           // Protected by L
	progressTime   float64        // Protected by L
	currentCommand *Command       // Protected by L
	commandCounter uint           // Protected by L
	currentOutput  *OutputLine    // Protected by L

	// Fields used by getStats but shared with Run
	statCond   *sync.Cond // Used by the main loop to wait for stat reception
	statActive bool
	statErr    error
	statRecord bool // Protected by statCond.L

	// Output channels
	statChan   chan Stat           // Nonblocking write
	updateChan chan *TestUpdateMsg // Nonblocking write, may be nil
}

const (
	UpdateReasonOutput = iota
	UpdateReasonScore
	UpdateReasonCommandDone
)

type TestUpdateMsg struct {
	Test   *Test
	Reason int
	Data   interface{}
}

// Statuses for commands
const (
	COMMAND_STATUS_NONE      = "none"      // The command has not yet run
	COMMAND_STATUS_RUNNING   = "running"   // The command is running
	COMMAND_STATUS_CORRECT   = "correct"   // The command produced the expected output and did not crash
	COMMAND_STATUS_INCORRECT = "incorrect" // The command received some partial credit
)

type Command struct {
	// Mongo ID
	ID bson.ObjectId `yaml:"-" json:"id" bson:"_id,omitempty"`

	// Set during init
	Type          string         `json:"type"`
	PromptPattern *regexp.Regexp `json:"-" bson:"-"`
	Input         InputLine      `json:"input"`

	// Set during target init
	PointsAvailable uint `json:"points_avail" bson:"points_avail"`
	PointsEarned    uint `json:"points_earned" bson:"points_earned"`

	// Set during run init
	Panic          string `json:"panic"`
	expectedOutput []*ExpectedOutputLine

	// Set during testing
	Output       []*OutputLine `json:"output"`
	SummaryStats Stat          `json:"summarystats"`
	AllStats     []Stat        `json:"stats"`

	// Set during evaluation
	Status string `json:"status"`

	// Backwards pointer to the Test
	test *Test
}

type InputLine struct {
	WallTime TimeFixedPoint `json:"walltime"`
	SimTime  TimeFixedPoint `json:"simtime"`
	Line     string         `json:"line"`
}

type OutputLine struct {
	WallTime TimeFixedPoint `json:"walltime"`
	SimTime  TimeFixedPoint `json:"simtime"`
	Buffer   bytes.Buffer   `json:"-" bson:"-"`
	Line     string         `json:"line"`
	Trusted  bool           `json:"trusted"`
	KeyName  string         `json:"keyname"`
}

type Status struct {
	WallTime TimeFixedPoint `json:"walltime"`
	SimTime  TimeFixedPoint `json:"simtime"`
	Status   string         `json:"status"`
	Message  string         `json:"message"`
}

type TimeFixedPoint float64

type TestResult string

const (
	TEST_RESULT_NONE      TestResult = "none"      // Hasn't run (initial status)
	TEST_RESULT_RUNNING   TestResult = "running"   // Running
	TEST_RESULT_CORRECT   TestResult = "correct"   // Met the output criteria
	TEST_RESULT_INCORRECT TestResult = "incorrect" // Possibly some partial points, but didn't complete everything successfully
	TEST_RESULT_ABORT     TestResult = "abort"     // Aborted - internal error
	TEST_RESULT_SKIP      TestResult = "skip"      // Skipped (dependency not met)
)

// MarshalJSON prints our TimeFixedPoint type as a fixed point float for JSON.
func (t TimeFixedPoint) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%.6f", t)), nil
}

// getTimeFixedPoint returns the current wall clock time as a TimeFixedPoint
func (t *Test) getWallTime() TimeFixedPoint {
	return TimeFixedPoint(float64(time.Now().UnixNano()-t.startTime) / float64(1000*1000*1000))
}

// Run a test161 test.
func (t *Test) Run(env *TestEnvironment) (err error) {
	// Serialize the current command state.
	t.L = &sync.Mutex{}

	// Save the test environment for other pieces that need it
	t.env = env

	defer func() {
		if env.Persistence != nil {
			env.Persistence.Notify(t, MSG_PERSIST_COMPLETE, 0)
		}
	}()

	t.Result = TEST_RESULT_RUNNING
	if env.Persistence != nil {
		env.Persistence.Notify(t, MSG_PERSIST_UPDATE, MSG_FIELD_STATUS)
	}

	// Set the instance-specific input and expected output
	for _, c := range t.Commands {
		if err = c.instantiate(env); err != nil {
			t.addStatus("aborted", "")
			t.Result = TEST_RESULT_ABORT
			return
		}
	}

	// Merge in test161 defaults for any missing configuration values
	err = t.MergeConf(CONF_DEFAULTS)
	if err != nil {
		t.addStatus("aborted", "")
		t.Result = TEST_RESULT_ABORT
		return err
	}

	// Create temp directory.
	tempRoot, err := ioutil.TempDir(t.Misc.TempDir, "test161")
	if err != nil {
		t.addStatus("aborted", "")
		t.Result = TEST_RESULT_ABORT
		return err
	}
	defer os.RemoveAll(tempRoot)
	t.tempDir = path.Join(tempRoot, "root")

	// Copy root.
	err = shutil.CopyTree(env.RootDir, t.tempDir, nil)
	if err != nil {
		t.addStatus("aborted", "")
		t.Result = TEST_RESULT_ABORT
		return err
	}

	// Make sure we have a kernel.
	kernelTarget := path.Join(t.tempDir, "kernel")
	_, err = os.Stat(kernelTarget)
	if err != nil {
		t.addStatus("aborted", "")
		t.Result = TEST_RESULT_ABORT
		return err
	}

	// Generate an alternate configuration to prevent collisions.
	confTarget := path.Join(t.tempDir, "test161.conf")
	t.ConfString, err = t.PrintConf()
	if err != nil {
		t.addStatus("aborted", "")
		t.Result = TEST_RESULT_ABORT
		return err
	}
	err = ioutil.WriteFile(confTarget, []byte(t.ConfString), 0440)
	if err != nil {
		t.addStatus("aborted", "")
		t.Result = TEST_RESULT_ABORT
		return err
	}
	if _, err := os.Stat(confTarget); os.IsNotExist(err) {
		t.addStatus("aborted", "")
		t.Result = TEST_RESULT_ABORT
		return err
	}

	// Create disks.
	if t.Sys161.Disk1.Enabled == "true" {
		create := exec.Command("disk161", "create", "LHD0.img", t.Sys161.Disk1.Bytes)
		create.Dir = t.tempDir
		err = create.Run()
		if err != nil {
			t.addStatus("aborted", "")
			t.Result = TEST_RESULT_ABORT
			return err
		}
	}
	if t.Sys161.Disk2.Enabled == "true" {
		create := exec.Command("disk161", "create", "LHD1.img", t.Sys161.Disk2.Bytes)
		create.Dir = t.tempDir
		err = create.Run()
		if err != nil {
			t.addStatus("aborted", "")
			t.Result = TEST_RESULT_ABORT
			return err
		}
	}

	// Coordinated with the getStat goroutine. I don't think that a channel
	// would work here.
	t.statCond = &sync.Cond{L: &sync.Mutex{}}

	// Initialize stat channel. Closed by getStats
	t.statChan = make(chan Stat)

	// Record stats during boot, but don't activate the monitor.
	t.statRecord = true

	// Set up the current command to point at boot
	t.commandCounter = 0
	t.currentCommand = t.Commands[t.commandCounter]
	t.currentCommand.Status = COMMAND_STATUS_RUNNING

	// Start sys161 and defer close.
	err = t.start161()
	if err != nil {
		t.addStatus("aborted", "")
		t.Result = TEST_RESULT_ABORT
		return err
	}
	defer t.stop161()
	t.addStatus("started", "")

	// Set up the output
	t.currentOutput = &OutputLine{}

	t.allCorrect = true

	for int(t.commandCounter) < len(t.Commands) {
		if t.commandCounter != 0 {
			t.currentCommand.Status = COMMAND_STATUS_RUNNING
			err = t.sendCommand(t.currentCommand.Input.Line + "\n")
			if err != nil {
				t.addStatus("expect", "couldn't send a command")
				break
			}
			statActive, statErr := t.enableStats()

			// If statErr is nil, getStats() exited cleanly, probably because sys161
			// shut down before we had a chance to expect anything.  However, we may
			// or may not have been expecting sys161 to shut down (panic vs q).
			// In that case, it's best to just keep going and handle the EOF below.
			if !statActive && statErr != nil {
				err = statErr
				break
			}
		}
		if t.currentCommand.PromptPattern == nil {
			// Wrap this so it doesn't fail. We don't really care about failures on
			// the shutdown path, and I have seen errors here in the regexp module.
			(func() {
				defer func() {
					_ = recover()
				}()
				t.sys161.ExpectEOF()
			})()
			t.addStatus("shutdown", "normal shutdown")
			err = nil
			break
		}
		match, expectErr := t.sys161.ExpectRegexp(t.currentCommand.PromptPattern)
		statActive, statErr := t.disableStats()

		eof := false

		// Handle timeouts, unexpected shutdowns, and other errors
		if expectErr == expect.ErrTimeout {
			t.addStatus("timeout", fmt.Sprintf("no prompt for %v s", t.Misc.PromptTimeout))
			break
		} else if expectErr == io.EOF || len(match.Groups) == 0 {
			// But is it reaaaally unexpected?
			if t.currentCommand.Panic == PANIC_NO {
				t.addStatus("shutdown", "unexpected shutdown")
				t.currentCommand.Status = COMMAND_STATUS_INCORRECT
				t.allCorrect = false
				t.currentCommand.PointsEarned = 0
				break
			} else {
				eof = true
			}
		} else if expectErr != nil {
			t.addStatus("expect", "")
			err = expectErr
			break
		} else if !statActive {
			err = statErr
			break
		}

		cur := t.finishCurCommand(env, eof)

		if cur.Status == COMMAND_STATUS_INCORRECT {
			t.allCorrect = false
		}

		// See if we can short-circuit the test
		if eof || cur.Panic != PANIC_NO {
			t.addStatus("shutdown", "expected panic")
			break
		} else if cur.Status == COMMAND_STATUS_INCORRECT {
			if t.ScoringMethod == "entire" {
				// No point in continuing, just shut down ungracefully.
				t.addStatus("shutdown", "short-circuit")
				break
			}
		} else if t.ScoringMethod == TEST_SCORING_PARTIAL {
			t.PointsEarned += cur.PointsEarned
		}
	}

	if uint(len(t.Commands)) > t.commandCounter {
		t.Commands = t.Commands[0 : t.commandCounter+1]
	}

	if err == nil {
		t.finishAndEvaluate()
	} else {
		t.Result = TEST_RESULT_ABORT
	}

	return err
}

func (t *Test) finishCurCommand(env *TestEnvironment, eof bool) *Command {

	t.L.Lock()
	defer t.L.Unlock()

	// Rotate running command to the next command, saving any previous
	// output as needed.
	if t.currentOutput.WallTime != 0.0 {
		t.currentOutput.Line = t.currentOutput.Buffer.String()
		t.outputLineComplete()
		t.currentCommand.Output = append(t.currentCommand.Output, t.currentOutput)

		if env.Persistence != nil {
			env.Persistence.Notify(t.currentCommand, MSG_PERSIST_UPDATE, MSG_FIELD_OUTPUT)
		}
	}

	cur := t.currentCommand
	cur.evaluate(env.KeyMap, eof)

	if env.Persistence != nil {
		env.Persistence.Notify(cur, MSG_PERSIST_UPDATE,
			MSG_FIELD_STATUS|MSG_FIELD_SCORE)
	}

	// Next line
	t.currentOutput = &OutputLine{}
	t.commandCounter++
	t.currentCommand = t.Commands[t.commandCounter]

	return cur
}

func (t *Test) finishAndEvaluate() {

	// Test Status
	if t.allCorrect {
		t.Result = TEST_RESULT_CORRECT
		if t.ScoringMethod == TEST_SCORING_ENTIRE {
			t.PointsEarned = t.PointsAvailable
		} else {
			// The partial points were computed along the way
		}
	} else {
		t.Result = TEST_RESULT_INCORRECT
	}
}

func (t *Test) sendUpdateMsg(reason int) {
	msg := &TestUpdateMsg{
		Test:   t,
		Reason: reason,
	}

	if reason == UpdateReasonOutput {
		msg.Data = t.currentOutput
	} else if reason == UpdateReasonCommandDone {
		msg.Data = t.currentCommand
	}

	select {
	case t.updateChan <- msg:
	default:
	}
}

// sendCommand sends a command persistently. All the retry logic to deal with
// dropped characters is now here.
func (t *Test) sendCommand(commandLine string) error {

	// If t.Misc.CharacterTimeout is set to zero disable the character retry
	// logic

	if t.Misc.RetryCharacters == "false" {
		t.sys161.Send(commandLine)
	} else {
		// Temporarily lower the expect timeout.
		t.sys161.SetTimeout(time.Duration(t.Misc.CharacterTimeout) * time.Millisecond)
		defer t.sys161.SetTimeout(time.Duration(t.Misc.PromptTimeout) * time.Second)

		for _, character := range commandLine {
			retryCount := uint(0)
			for ; retryCount < t.Misc.CommandRetries; retryCount++ {
				err := t.sys161.Send(string(character))
				if err != nil {
					return err
				}
				_, err = t.sys161.ExpectRegexp(regexp.MustCompile(regexp.QuoteMeta(string(character))))
				if err == nil {
					break
				} else if err == expect.ErrTimeout {
					continue
				} else {
					return err
				}
			}
			if retryCount == t.Misc.CommandRetries {
				return errors.New("test161: timeout sending command")
			}
		}
	}
	return nil
}

// start161 is a private helper function to start the sys161 expect process.
func (t *Test) start161() error {
	// Disable debugger connections on panic and set our alternate
	// configuration.
	sys161Path := t.Sys161.Path
	if strings.HasPrefix(t.Sys161.Path, "./") {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		sys161Path = path.Join(cwd, sys161Path)
	}
	run := exec.Command(sys161Path, "-X", "-c", "test161.conf", "kernel")
	run.Dir = t.tempDir
	pty, err := pty.Start(run)
	if err != nil {
		return err
	}

	// Get serious about killing things.
	var killer func()
	if t.Misc.KillOnExit == "true" {
		killer = func() {
			run.Process.Signal(os.Kill)
		}
	} else {
		killer = func() {
			run.Process.Kill()
		}
	}

	// Set timeout at create to avoid hanging with early failures.
	t.L.Lock()
	t.running = true
	t.L.Unlock()
	t.statCond.L.Lock()
	t.statActive = true
	t.statCond.L.Unlock()
	t.sys161 = expect.Create(pty, killer, t, time.Duration(t.Misc.PromptTimeout)*time.Second)
	t.startTime = time.Now().UnixNano()

	return nil
}

func (t *Test) stop161() {
	t.L.Lock()
	wasRunning := t.running
	if wasRunning {
		t.running = false
		t.WallTime = t.getWallTime()
	}
	t.L.Unlock()
	if wasRunning {
		t.sys161.Close()
	}
}

func (t *Test) addStatus(status string, message string) {
	t.L.Lock()
	t.Status = append(t.Status, Status{
		WallTime: t.getWallTime(),
		SimTime:  t.SimTime,
		Status:   status,
		Message:  message,
	})
	t.L.Unlock()
}

// Split a command line into its base command and args.
func (l *InputLine) splitCommand() (prefix, base string, args []string) {
	command := l.Line
	var pos, start int = 0, 0
	var inQuote, escape = false, false
	args = make([]string, 0)
	prefix = ""

	// Special case: "p <command>"
	if strings.HasPrefix(command, "p ") {
		prefix = "p"
		start, pos = 0, 2
	}

	// We're looking for the first unescaped space that isn't in quotes,
	// or the end of the string.
	for pos < len(command) {
		if command[pos] == '"' && !escape {
			inQuote = !inQuote
		} else if escape || command[pos] == '\\' {
			escape = !escape
		} else if !inQuote && command[pos] == ' ' {
			// We have the command/next arg
			args = append(args, command[start:pos])
			start = pos + 1 //skip the space
		}
		pos++
	}

	// Add the last argument
	if start < len(command) {
		args = append(args, command[start:len(command)])
	}

	base = args[0]
	if len(args) == 1 {
		args = nil
	} else {
		args = args[1:]
	}

	return
}

func (l *InputLine) replaceArgs(args []string) {
	prefix, base, _ := l.splitCommand()

	if len(prefix) > 0 {
		l.Line = prefix + " "
	}

	l.Line += base

	for _, a := range args {
		l.Line += " " + a
	}
}

// Partial credit regular expression
var partialCreditExp *regexp.Regexp = regexp.MustCompile(`^PARTIAL CREDIT ([0-9]+) OF ([0-9]+)$`)

// Evaluate a single command, setting its status and points
func (c *Command) evaluate(keyMap map[string]string, eof bool) {
	c.PointsEarned = 0

	if c.Panic == PANIC_YES && !eof {
		// Not correct, we should have panicked
		c.Status = COMMAND_STATUS_INCORRECT
		return
	} else if len(c.expectedOutput) == 0 {
		// If we didn't crash and we aren't expecting anything, then
		// we passed with flying colors.
		c.PointsEarned = c.PointsAvailable
		c.Status = COMMAND_STATUS_CORRECT
		return
	}

	// We're expecting something. First check if we got exactly what we're
	// looking for (it's OK if there are extra output lines).
	expectedIndex, actualIndex := 0, 0
	for actualIndex < len(c.Output) && expectedIndex < len(c.expectedOutput) {
		expected := c.expectedOutput[expectedIndex]
		actual := c.Output[actualIndex]

		if actual.Line == expected.Text {
			// We only count this as a match if the message is verified or we don't
			// care about keys.  The latter happens if the command specifically tells
			// us that, or the keyMap is empty - which happens on the client side.
			_, hasKey := keyMap[expected.KeyName]
			if !expected.Trusted || !hasKey || (actual.Trusted && actual.KeyName == expected.KeyName) {
				expectedIndex++
			}
		}
		actualIndex++
	}

	// If we've matched all expected lines, the command succeeded and full
	// points are awarded (if there are any).
	if expectedIndex == len(c.expectedOutput) {
		c.Status = COMMAND_STATUS_CORRECT
		c.PointsEarned = c.PointsAvailable
	} else {
		// The result is incorrect, but there still be some partial credit
		c.Status = COMMAND_STATUS_INCORRECT
		c.PointsEarned = 0

		totalEarned, totalAvail := 0, 0

		id := c.Id()
		_, hasKey := keyMap[id]

		for _, line := range c.Output {
			// Only check trusted lines signed with our key
			if !hasKey || (line.Trusted && line.KeyName == id) {
				if res := partialCreditExp.FindStringSubmatch(line.Line); len(res) == 3 {
					if earned, err := strconv.Atoi(res[1]); err != nil && earned > 0 {
						if avail, err := strconv.Atoi(res[2]); err != nil && avail > 0 {
							totalAvail += avail
							totalEarned += earned
						}
					}
				}
			}
		}
		if totalEarned > 0 && totalAvail > 0 {
			// Integral points only
			c.PointsEarned = uint(float32(c.PointsAvailable) * (float32(totalEarned) / float32(totalAvail)))
		}

		// If they got all of the partial credit, this command was actually correct
		if c.PointsEarned == c.PointsAvailable {
			c.Status = COMMAND_STATUS_CORRECT
		}
	}
}

// Regexp corresponding to the kernel's secprintf. The meaning is
// (id, hash, salt, message).
var os161Secure *regexp.Regexp = regexp.MustCompile(`^\((.*), ([0-9a-f]*), ([0-9a-f].*), (.*)\)$`)

func (t *Test) outputLineComplete() {

	line := t.currentOutput

	// First, strip off the "\r\r\n"
	pos := len(line.Line) - 1
	for pos >= 0 {
		if line.Line[pos] != '\r' && line.Line[pos] != '\n' {
			break
		}
		pos -= 1
	}

	if pos == 0 {
		line.Line = ""
		return
	}

	line.Line = line.Line[0 : pos+1]

	// Next, check if this is a secure line
	if res := os161Secure.FindStringSubmatch(line.Line); len(res) == 5 {
		// The message has the secprintf form, now verify that we trust this message.
		//If we do, note the key and only output the payload.
		id := res[1]
		hash := res[2]
		salt := res[3]
		key := t.env.KeyMap[id] + salt

		mac := hmac.New(sha256.New, []byte(key))
		mac.Write([]byte(res[4]))
		expected := strings.ToLower(hex.EncodeToString(mac.Sum(nil)))

		if expected == hash {
			line.Trusted = true
			line.KeyName = id
			line.Line = res[4]
		}
	}

}
