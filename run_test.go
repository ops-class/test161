package test161

import (
	"flag"
	"fmt"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"
)

var TEST_DEFAULTS = Test{
	Stat: StatConf{
		Resolution: 0.01,
		Window:     100,
	},
	Misc: MiscConf{
		PromptTimeout: 40.0,
	},
}

var defaultEnv *TestEnvironment = nil
var testFlagDB = false

func init() {
	// Make sure the default test manager exists first
	var err error
	defaultEnv, err = NewEnvironment("./fixtures", &DoNothingPersistence{})

	if err != nil {
		panic(fmt.Sprintf("Unable to create default environment: %v", err))
	}

	defaultEnv.TestDir = "./fixtures/tests/nocycle"
	defaultEnv.RootDir = "./fixtures/root"

	// Command line flags
	flag.BoolVar(&testFlagDB, "db", false, "Run tests that rely on mongodb")

	// No key map, cache, etc.
}

func TestMain(m *testing.M) {
	rand.Seed(time.Now().UTC().UnixNano())
	os.Exit(m.Run())
}

func TestRunBoot(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("q")
	assert.Nil(err)
	assert.Nil(test.MergeConf(TEST_DEFAULTS))
	assert.Nil(test.Run(defaultEnv))

	assert.Equal(len(test.Commands), 2)
	if len(test.Commands) == 2 {
		assert.Equal(test.Commands[0].Type, "kernel")
		assert.Equal(test.Commands[0].Input.Line, "boot")
		assert.Equal(test.Commands[1].Type, "kernel")
		assert.Equal(test.Commands[1].Input.Line, "q")
	}

	assert.Equal(len(test.Status), 2)
	if len(test.Status) == 2 {
		assert.Equal(test.Status[0].Status, "started")
		assert.Equal(test.Status[1].Status, "shutdown")
		assert.True(strings.HasPrefix(test.Status[1].Message, "normal"))
	}

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestRunShell(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("$ /bin/true")
	assert.Nil(err)
	assert.Nil(test.MergeConf(TEST_DEFAULTS))
	assert.Nil(test.Run(defaultEnv))

	assert.Equal(len(test.Commands), 5)
	if len(test.Commands) == 5 {
		assert.Equal(test.Commands[0].Type, "kernel")
		assert.Equal(test.Commands[0].Input.Line, "boot")
		assert.Equal(test.Commands[1].Type, "user")
		assert.Equal(test.Commands[1].Input.Line, "s")
		assert.Equal(test.Commands[2].Type, "user")
		assert.Equal(test.Commands[2].Input.Line, "/bin/true")
		assert.Equal(test.Commands[3].Type, "user")
		assert.Equal(test.Commands[3].Input.Line, "exit")
		assert.Equal(test.Commands[4].Type, "kernel")
		assert.Equal(test.Commands[4].Input.Line, "q")
	}

	assert.Equal(len(test.Status), 2)
	if len(test.Status) == 2 {
		assert.Equal(test.Status[0].Status, "started")
		assert.Equal(test.Status[1].Status, "shutdown")
		assert.True(strings.HasPrefix(test.Status[1].Message, "normal"))
	}

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestRunPanic(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("panic")
	assert.Nil(err)
	assert.Nil(test.MergeConf(TEST_DEFAULTS))
	test.Monitor.Enabled = "false"
	assert.Nil(test.Run(defaultEnv))

	assert.Equal(len(test.Commands), 2)
	if len(test.Commands) == 2 {
		assert.Equal(test.Commands[0].Type, "kernel")
		assert.Equal(test.Commands[0].Input.Line, "boot")
		assert.Equal(test.Commands[1].Type, "kernel")
		assert.Equal(test.Commands[1].Input.Line, "panic")
	}

	assert.Equal(len(test.Status), 2)
	if len(test.Status) == 2 {
		assert.Equal(test.Status[0].Status, "started")
		assert.Equal(test.Status[1].Status, "shutdown")
		assert.True(strings.HasPrefix(test.Status[1].Message, "unexpected"))
	}

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestRunShll(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString(`---
commandconf:
  - prefix: "!"
    prompt: "OS/161$ "
    start: $ /testbin/shll -p 30
    end: exit
---
! exit
`)
	assert.Nil(err)
	assert.Nil(test.MergeConf(TEST_DEFAULTS))
	test.Monitor.User.EnableMin = "false"
	test.Monitor.Kernel.EnableMin = "false"
	test.Misc.CommandRetries = 20
	assert.Nil(test.Run(defaultEnv))

	assert.Equal(len(test.Commands), 6)
	if len(test.Commands) == 6 {
		assert.Equal(test.Commands[0].Type, "kernel")
		assert.Equal(test.Commands[0].Input.Line, "boot")
		assert.Equal(test.Commands[1].Type, "user")
		assert.Equal(test.Commands[1].Input.Line, "s")
		assert.Equal(test.Commands[2].Type, "user")
		assert.Equal(test.Commands[2].Input.Line, "/testbin/shll -p 30")
		assert.Equal(test.Commands[3].Type, "user")
		assert.Equal(test.Commands[3].Input.Line, "exit")
		assert.Equal(test.Commands[4].Type, "user")
		assert.Equal(test.Commands[4].Input.Line, "exit")
		assert.Equal(test.Commands[5].Type, "kernel")
		assert.Equal(test.Commands[5].Input.Line, "q")
	}

	assert.Equal(len(test.Status), 2)
	if len(test.Status) == 2 {
		assert.Equal(test.Status[0].Status, "started")
		assert.Equal(test.Status[1].Status, "shutdown")
		assert.True(strings.HasPrefix(test.Status[1].Message, "normal"))
	}

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestRunShllLossy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping shlllossy test in short mode")
	}
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString(`---
commandconf:
  - prefix: "!"
    prompt: "OS/161$ "
    start: $ /testbin/shll -p 50
    end: exit
---
! exit
`)
	assert.Nil(err)
	assert.Nil(test.MergeConf(TEST_DEFAULTS))
	test.Monitor.User.EnableMin = "false"
	test.Monitor.Kernel.EnableMin = "false"
	test.Misc.RetryCharacters = "false"
	test.Monitor.ProgressTimeout = 1.0
	test.Misc.KillOnExit = "false"
	assert.Nil(test.Run(defaultEnv))

	assert.NotEqual(len(test.Commands), 6)

	assert.Equal(len(test.Status), 3)
	if len(test.Status) == 3 {
		assert.Equal(test.Status[0].Status, "started")
		assert.Equal(test.Status[1].Status, "monitor")
		assert.True(strings.HasPrefix(test.Status[1].Message, "no progress"))
		assert.Equal(test.Status[2].Status, "shutdown")
		assert.True(strings.HasPrefix(test.Status[2].Message, "unexpected"))
	}

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestRunResults(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	// Shell - should return OK
	test, err := TestFromString("$ /bin/true")
	assert.Nil(err)
	assert.Nil(test.MergeConf(TEST_DEFAULTS))
	err = test.Run(defaultEnv)
	assert.Nil(err)
	assert.Equal(TEST_RESULT_CORRECT, test.Result)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())

	// Panic - should return FAIL
	test, err = TestFromString("panic")
	assert.Nil(err)
	assert.Nil(test.MergeConf(TEST_DEFAULTS))
	test.Monitor.Enabled = "false"
	err = test.Run(defaultEnv)
	assert.Nil(err)
	assert.Equal(TEST_RESULT_INCORRECT, test.Result)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestRunBadSys161(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString(`---
sys161:
  path: "./fixtures/sys161/sys161-2.0.5"
---
q
`)
	assert.Nil(err)
	assert.Nil(test.MergeConf(TEST_DEFAULTS))
	assert.Nil(test.Run(defaultEnv))

	assert.Equal(len(test.Status), 3)
	if len(test.Status) == 3 {
		assert.Equal(test.Status[0].Status, "started")
		assert.Equal(test.Status[1].Status, "stats")
		assert.True(strings.HasPrefix(test.Status[1].Message, "incorrect stat format"))
		assert.Equal(test.Status[2].Status, "shutdown")
		assert.True(strings.HasPrefix(test.Status[2].Message, "unexpected"))
	}

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

/////////////////    Grading tests   /////////////////

// Create a fake command complete with output lines.
func commandFromOutput(test *Test, cmd, output string) *Command {

	for _, c := range test.Commands {
		if c.Input.Line == cmd {
			lines := strings.Split(output, "\n")
			for _, l := range lines {
				test.currentOutput = &OutputLine{
					Line: l,
				}
				test.outputLineComplete()
				c.Output = append(c.Output, test.currentOutput)
			}
			return c
		}
	}

	return nil
}

func TestRunGradingCorrectOutput(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	// Forktest with correct, unsecured output
	test, err := TestFromString("p /testbin/forktest")
	test.env = defaultEnv
	if err != nil {
		t.FailNow()
		t.Log(err)
	}

	assert.NotNil(t)

	c := commandFromOutput(test, "p /testbin/forktest", `/testbin/forktest: Starting. Expect this many:
|----------------------------|
AABBBBCCCCCDCDDDDCDDDCDDDDDDDD
/testbin/forktest: SUCCESS
/testbin/forktest: Complete.
Program (pid 2) exited with status 0
Operation took 1.215512161 seconds
`)

	if c == nil {
		t.Log("Command not found in Test")
		t.FailNow()
	}

	err = c.Instantiate(defaultEnv)
	assert.Nil(err)
	assert.True(len(c.ExpectedOutput) > 0)

	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_CORRECT, c.Status)

	// Correct output, but an unexpected exit
	c.Status = COMMAND_STATUS_RUNNING
	c.evaluate(nil, true)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)

	// Correct output, with an expected exit
	c.Panic = CMD_OPT_YES
	c.Status = COMMAND_STATUS_RUNNING
	c.evaluate(nil, true)
	assert.Equal(COMMAND_STATUS_CORRECT, c.Status)
	c.Panic = CMD_OPT_MAYBE
	c.Status = COMMAND_STATUS_RUNNING
	c.evaluate(nil, true)
	assert.Equal(COMMAND_STATUS_CORRECT, c.Status)

	// Expected panic that we don't get
	c.Panic = CMD_OPT_YES
	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)

	c.Panic = CMD_OPT_NO

	// Same as panic, but with a timeout
	c.TimesOut = CMD_OPT_YES
	c.TimedOut = true
	c.Status = COMMAND_STATUS_RUNNING
	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_CORRECT, c.Status)
	c.TimesOut = CMD_OPT_MAYBE
	c.Status = COMMAND_STATUS_RUNNING
	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_CORRECT, c.Status)

	c.TimesOut = CMD_OPT_YES
	c.TimedOut = false
	c.Status = COMMAND_STATUS_RUNNING
	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)
}

func TestRunGradingIncorrectOutput(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	// This is similar to the correct output case, but everything should fail

	// Forktest with correct, unsecured output
	test, err := TestFromString("p /testbin/forktest")
	test.env = defaultEnv
	if err != nil {
		t.FailNow()
		t.Log(err)
	}

	assert.NotNil(t)

	c := commandFromOutput(test, "p /testbin/forktest", `/testbin/forktest: Starting. Expect this many:
|----------------------------|
AABBBBCCCCCDCDDDDCDDDCDDDDDDDD
/testbin/forktest: SUCCESSSSSSS
/testbin/forktest: Complete.
Program (pid 2) exited with status 0
Operation took 1.215512161 seconds
`)

	if c == nil {
		t.Log("Command not found in Test")
		t.FailNow()
	}

	err = c.Instantiate(defaultEnv)
	assert.Nil(err)
	assert.True(len(c.ExpectedOutput) > 0)

	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)

	// Correct output, but an unexpected exit
	c.Status = COMMAND_STATUS_RUNNING
	c.evaluate(nil, true)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)

	// Correct output, with an expected exit
	c.Panic = CMD_OPT_YES
	c.Status = COMMAND_STATUS_RUNNING
	c.evaluate(nil, true)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)
	c.Panic = CMD_OPT_MAYBE
	c.Status = COMMAND_STATUS_RUNNING
	c.evaluate(nil, true)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)

	// Expected panic that we don't get
	c.Panic = CMD_OPT_YES
	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)

	c.Panic = CMD_OPT_NO

	// Same as panic, but with a timeout
	c.TimesOut = CMD_OPT_YES
	c.TimedOut = true
	c.Status = COMMAND_STATUS_RUNNING
	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)
	c.TimesOut = CMD_OPT_MAYBE
	c.Status = COMMAND_STATUS_RUNNING
	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)

	c.TimesOut = CMD_OPT_YES
	c.TimedOut = false
	c.Status = COMMAND_STATUS_RUNNING
	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)
}

func TestRunGradingPartialCredit(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	// This is similar to the correct output case, but everything should fail

	// Forktest with correct, unsecured output
	test, err := TestFromString("p /testbin/forktest")
	test.env = defaultEnv
	if err != nil {
		t.FailNow()
		t.Log(err)
	}

	assert.NotNil(t)

	c := commandFromOutput(test, "p /testbin/forktest", `/testbin/forktest: Starting. Expect this many:
|----------------------------|
AABBBBCCCCCDCDDDDCDDDCDDDDDDDD
/testbin/forktest: PARTIAL CREDIT 6 OF 10
/testbin/forktest: Complete.
Program (pid 2) exited with status 0
Operation took 1.215512161 seconds
`)

	if c == nil {
		t.Log("Command not found in Test")
		t.FailNow()
	}

	err = c.Instantiate(defaultEnv)
	assert.Nil(err)
	assert.True(len(c.ExpectedOutput) > 0)

	c.PointsAvailable = 5

	// Partial credit is never correct unless they get all the points
	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)
	assert.Equal(uint(3), c.PointsEarned)

	// Correct output, but an unexpected exit
	c.Status = COMMAND_STATUS_RUNNING
	c.PointsEarned = 0
	c.evaluate(nil, true)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)
	assert.Equal(uint(0), c.PointsEarned)

	// Correct output, with an expected exit
	c.Panic = CMD_OPT_YES
	c.Status = COMMAND_STATUS_RUNNING
	c.PointsEarned = 0
	c.evaluate(nil, true)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)
	assert.Equal(uint(3), c.PointsEarned)

	c.Panic = CMD_OPT_MAYBE
	c.Status = COMMAND_STATUS_RUNNING
	c.evaluate(nil, true)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)
	assert.Equal(uint(3), c.PointsEarned)

	// Expected panic that we don't get
	c.Panic = CMD_OPT_YES
	c.PointsEarned = 0
	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)
	assert.Equal(uint(0), c.PointsEarned)

	c.Panic = CMD_OPT_NO

	// Same as panic, but with a timeout
	c.TimesOut = CMD_OPT_YES
	c.TimedOut = true
	c.Status = COMMAND_STATUS_RUNNING
	c.PointsEarned = 0
	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)
	assert.Equal(uint(3), c.PointsEarned)

	c.TimesOut = CMD_OPT_MAYBE
	c.Status = COMMAND_STATUS_RUNNING
	c.PointsEarned = 0
	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)
	assert.Equal(uint(3), c.PointsEarned)

	// Didn't get the timeout
	c.PointsEarned = 0
	c.TimesOut = CMD_OPT_YES
	c.TimedOut = false
	c.Status = COMMAND_STATUS_RUNNING
	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)
	assert.Equal(uint(0), c.PointsEarned)
}

func TestRunGradingFullPartialCredit(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	// This is similar to the correct output case, but everything should fail

	// Forktest with correct, unsecured output
	test, err := TestFromString("p /testbin/forktest")
	test.env = defaultEnv
	if err != nil {
		t.FailNow()
		t.Log(err)
	}

	assert.NotNil(t)

	c := commandFromOutput(test, "p /testbin/forktest", `/testbin/forktest: Starting. Expect this many:
|----------------------------|
AABBBBCCCCCDCDDDDCDDDCDDDDDDDD
/testbin/forktest: PARTIAL CREDIT 10 OF 10
/testbin/forktest: Complete.
Program (pid 2) exited with status 0
Operation took 1.215512161 seconds
`)

	if c == nil {
		t.Log("Command not found in Test")
		t.FailNow()
	}

	err = c.Instantiate(defaultEnv)
	assert.Nil(err)
	assert.True(len(c.ExpectedOutput) > 0)

	c.PointsAvailable = 5

	// Getting all partial credit -> correct
	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_CORRECT, c.Status)
	assert.Equal(uint(5), c.PointsEarned)

	// Correct output, but an unexpected exit
	c.Status = COMMAND_STATUS_RUNNING
	c.PointsEarned = 0
	c.evaluate(nil, true)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)
	assert.Equal(uint(0), c.PointsEarned)

	// Correct output, with an expected exit
	c.Panic = CMD_OPT_YES
	c.Status = COMMAND_STATUS_RUNNING
	c.PointsEarned = 0
	c.evaluate(nil, true)
	assert.Equal(COMMAND_STATUS_CORRECT, c.Status)
	assert.Equal(uint(5), c.PointsEarned)

	c.Panic = CMD_OPT_MAYBE
	c.Status = COMMAND_STATUS_RUNNING
	c.evaluate(nil, true)
	assert.Equal(COMMAND_STATUS_CORRECT, c.Status)
	assert.Equal(uint(5), c.PointsEarned)

	// Expected panic that we don't get
	c.Panic = CMD_OPT_YES
	c.PointsEarned = 0
	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)
	assert.Equal(uint(0), c.PointsEarned)

	c.Panic = CMD_OPT_NO

	// Same as panic, but with a timeout
	c.TimesOut = CMD_OPT_YES
	c.TimedOut = true
	c.Status = COMMAND_STATUS_RUNNING
	c.PointsEarned = 0
	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_CORRECT, c.Status)
	assert.Equal(uint(5), c.PointsEarned)

	c.TimesOut = CMD_OPT_MAYBE
	c.Status = COMMAND_STATUS_RUNNING
	c.PointsEarned = 0
	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_CORRECT, c.Status)
	assert.Equal(uint(5), c.PointsEarned)

	// Didn't get the timeout
	c.PointsEarned = 0
	c.TimesOut = CMD_OPT_YES
	c.TimedOut = false
	c.Status = COMMAND_STATUS_RUNNING
	c.evaluate(nil, false)
	assert.Equal(COMMAND_STATUS_INCORRECT, c.Status)
	assert.Equal(uint(0), c.PointsEarned)
}
