package test161

import (
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
		PromptTimeout: 30.0,
	},
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
	assert.Nil(test.Run("./fixtures/"))

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
	assert.Nil(test.Run("./fixtures/"))

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
	assert.Nil(test.Run("./fixtures/"))

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
	assert.Nil(test.Run("./fixtures/"))

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
	assert.Nil(test.Run("./fixtures/"))

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
	err = test.Run("./fixtures")
	assert.Nil(err)
	assert.Equal(T_RES_OK, test.Result)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())

	// Panic - should return FAIL
	test, err = TestFromString("panic")
	assert.Nil(err)
	assert.Nil(test.MergeConf(TEST_DEFAULTS))
	test.Monitor.Enabled = "false"
	err = test.Run("./fixtures")
	assert.Nil(err)
	assert.Equal(T_RES_FAIL, test.Result)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}
