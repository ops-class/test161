package test161

import (
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

func TestStatsKernelDeadlock(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("dl")
	assert.Nil(err)
	assert.Nil(test.MergeConf(TEST_DEFAULTS))
	test.Monitor.Enabled = "true"
	test.Monitor.ProgressTimeout = 8.0
	assert.Nil(test.Run(defaultEnv))

	assert.Equal(len(test.Commands), 2)
	if len(test.Commands) == 2 {
		assert.Equal(test.Commands[0].Type, "kernel")
		assert.Equal(test.Commands[0].Input.Line, "boot")
		assert.Equal(test.Commands[1].Type, "kernel")
		assert.Equal(test.Commands[1].Input.Line, "dl")
	}

	assert.Equal(len(test.Status), 3)
	if len(test.Status) == 3 {
		assert.Equal(test.Status[0].Status, "started")
		assert.Equal(test.Status[1].Status, "monitor")
		assert.True(strings.HasPrefix(test.Status[1].Message, "insufficient kernel instructions"))
		assert.Equal(test.Status[2].Status, "shutdown")
		assert.True(strings.HasPrefix(test.Status[2].Message, "unexpected"))
	}

	assert.True(test.SimTime < 8.0)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestStatsKernelLivelock(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("ll16")
	assert.Nil(err)
	assert.Nil(test.MergeConf(TEST_DEFAULTS))
	test.Monitor.Enabled = "true"
	test.Sys161.CPUs = 1
	test.Monitor.ProgressTimeout = 8.0
	assert.Nil(test.Run(defaultEnv))

	assert.Equal(len(test.Commands), 2)
	if len(test.Commands) == 2 {
		assert.Equal(test.Commands[0].Type, "kernel")
		assert.Equal(test.Commands[0].Input.Line, "boot")
		assert.Equal(test.Commands[1].Type, "kernel")
		assert.Equal(test.Commands[1].Input.Line, "ll16")
	}

	assert.Equal(len(test.Status), 3)
	if len(test.Status) == 3 {
		assert.Equal(test.Status[0].Status, "started")
		assert.Equal(test.Status[1].Status, "monitor")
		assert.True(strings.HasPrefix(test.Status[1].Message, "too many kernel instructions"))
		assert.Equal(test.Status[2].Status, "shutdown")
		assert.True(strings.HasPrefix(test.Status[2].Message, "unexpected"))
	}

	assert.True(test.SimTime < 8.0)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestStatsUserDeadlock(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("p /testbin/waiter")
	assert.Nil(err)
	assert.Nil(test.MergeConf(TEST_DEFAULTS))
	test.Monitor.Enabled = "true"
	test.Monitor.Kernel.EnableMin = "false"
	test.Misc.PromptTimeout = 8.0
	assert.Nil(test.Run(defaultEnv))

	assert.Equal(len(test.Commands), 2)
	if len(test.Commands) == 2 {
		assert.Equal(test.Commands[0].Type, "kernel")
		assert.Equal(test.Commands[0].Input.Line, "boot")
		assert.Equal(test.Commands[1].Type, "user")
		assert.Equal(test.Commands[1].Input.Line, "p /testbin/waiter")
	}

	assert.Equal(len(test.Status), 3)
	if len(test.Status) == 3 {
		assert.Equal(test.Status[0].Status, "started")
		assert.Equal(test.Status[1].Status, "monitor")
		assert.True(strings.HasPrefix(test.Status[1].Message, "insufficient user instructions"))
		assert.Equal(test.Status[2].Status, "shutdown")
		assert.True(strings.HasPrefix(test.Status[2].Message, "unexpected"))
	}

	assert.True(test.SimTime < 8.0)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestStatsKernelProgress(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("ll1")
	assert.Nil(err)
	assert.Nil(test.MergeConf(TEST_DEFAULTS))
	test.Monitor.Enabled = "true"
	test.Monitor.Kernel.EnableMin = "false"
	test.Monitor.User.EnableMin = "false"
	test.Monitor.ProgressTimeout = 2.0
	test.Misc.PromptTimeout = 10.0
	assert.Nil(test.Run(defaultEnv))

	assert.Equal(len(test.Commands), 2)
	if len(test.Commands) == 2 {
		assert.Equal(test.Commands[0].Type, "kernel")
		assert.Equal(test.Commands[0].Input.Line, "boot")
		assert.Equal(test.Commands[1].Type, "kernel")
		assert.Equal(test.Commands[1].Input.Line, "ll1")
	}

	assert.Equal(len(test.Status), 3)
	if len(test.Status) == 3 {
		assert.Equal(test.Status[0].Status, "started")
		assert.Equal(test.Status[1].Status, "monitor")
		assert.True(strings.HasPrefix(test.Status[1].Message, "no progress"))
		assert.Equal(test.Status[2].Status, "shutdown")
		assert.True(strings.HasPrefix(test.Status[2].Message, "unexpected"))
	}

	assert.True(test.SimTime < 6.0)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestStatsUserProgress(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("p /testbin/waiter")
	assert.Nil(err)
	assert.Nil(test.MergeConf(TEST_DEFAULTS))
	test.Monitor.Enabled = "true"
	test.Monitor.Kernel.EnableMin = "false"
	test.Monitor.User.EnableMin = "false"
	test.Monitor.ProgressTimeout = 2.0
	test.Misc.PromptTimeout = 10.0
	assert.Nil(test.Run(defaultEnv))

	assert.Equal(len(test.Commands), 2)
	if len(test.Commands) == 2 {
		assert.Equal(test.Commands[0].Type, "kernel")
		assert.Equal(test.Commands[0].Input.Line, "boot")
		assert.Equal(test.Commands[1].Type, "user")
		assert.Equal(test.Commands[1].Input.Line, "p /testbin/waiter")
	}

	assert.Equal(len(test.Status), 3)
	if len(test.Status) == 3 {
		assert.Equal(test.Status[0].Status, "started")
		assert.Equal(test.Status[1].Status, "monitor")
		assert.True(strings.HasPrefix(test.Status[1].Message, "no progress"))
		assert.Equal(test.Status[2].Status, "shutdown")
		assert.True(strings.HasPrefix(test.Status[2].Message, "unexpected"))
	}

	assert.True(test.SimTime < 6.0)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}
