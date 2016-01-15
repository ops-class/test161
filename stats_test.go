package test161

import (
	"github.com/stretchr/testify/assert"
	//"strings"
	"testing"
)

func TestStatsKernelDeadlock(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("dl")
	assert.Nil(err)
	assert.Nil(test.MergeConf(TEST_DEFAULTS))
	test.Monitor.ProgressTimeout = 8.0
	assert.Nil(test.Run("./fixtures/"))

	assert.Equal(len(test.Commands), 2)
	if len(test.Commands) == 2 {
		assert.Equal(test.Commands[0].Type, "kernel")
		assert.Equal(test.Commands[0].Input.Line, "boot")
		assert.Equal(test.Commands[1].Type, "kernel")
		assert.Equal(test.Commands[1].Input.Line, "dl")
	}

	/*
		assert.Equal(test.Status, "monitor")
		assert.True(strings.HasPrefix(test.ShutdownMessage, "insufficient kernel cycles"))
	*/
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
	test.Monitor.ProgressTimeout = 8.0
	assert.Nil(test.Run("./fixtures/"))

	assert.Equal(len(test.Commands), 2)
	if len(test.Commands) == 2 {
		assert.Equal(test.Commands[0].Type, "kernel")
		assert.Equal(test.Commands[0].Input.Line, "boot")
		assert.Equal(test.Commands[1].Type, "kernel")
		assert.Equal(test.Commands[1].Input.Line, "ll16")
	}

	/*
		assert.Equal(test.Status, "monitor")
		assert.True(strings.HasPrefix(test.ShutdownMessage, "too many kernel cycles"))
	*/
	assert.True(test.SimTime < 8.0)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestStatsUserDeadlock(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("$ /testbin/waiter")
	assert.Nil(err)
	assert.Nil(test.MergeConf(TEST_DEFAULTS))
	test.Monitor.Kernel.EnableMin = "false"
	test.Misc.PromptTimeout = 8.0
	assert.Nil(test.Run("./fixtures/"))

	assert.Equal(len(test.Commands), 3)
	if len(test.Commands) == 3 {
		assert.Equal(test.Commands[0].Type, "kernel")
		assert.Equal(test.Commands[0].Input.Line, "boot")
		assert.Equal(test.Commands[1].Type, "user")
		assert.Equal(test.Commands[1].Input.Line, "s")
		assert.Equal(test.Commands[2].Type, "user")
		assert.Equal(test.Commands[2].Input.Line, "/testbin/waiter")
	}

	/*
		assert.Equal(test.Status, "monitor")
		assert.True(strings.HasPrefix(test.ShutdownMessage, "insufficient user cycles"))
	*/
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
	test.Monitor.Kernel.EnableMin = "false"
	test.Monitor.User.EnableMin = "false"
	test.Monitor.ProgressTimeout = 1.0
	test.Misc.PromptTimeout = 10.0
	assert.Nil(test.Run("./fixtures/"))

	assert.Equal(len(test.Commands), 2)
	if len(test.Commands) == 2 {
		assert.Equal(test.Commands[0].Type, "kernel")
		assert.Equal(test.Commands[0].Input.Line, "boot")
		assert.Equal(test.Commands[1].Type, "kernel")
		assert.Equal(test.Commands[1].Input.Line, "ll1")
	}

	/*
		assert.Equal(test.Status, "monitor")
		assert.True(strings.HasPrefix(test.ShutdownMessage, "no progress"))
	*/
	assert.True(test.SimTime < 4.0)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestStatsUserProgress(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("$ /testbin/waiter")
	assert.Nil(err)
	assert.Nil(test.MergeConf(TEST_DEFAULTS))
	test.Monitor.Kernel.EnableMin = "false"
	test.Monitor.User.EnableMin = "false"
	test.Monitor.ProgressTimeout = 1.0
	test.Misc.PromptTimeout = 10.0
	assert.Nil(test.Run("./fixtures/"))

	assert.Equal(len(test.Commands), 3)
	if len(test.Commands) == 3 {
		assert.Equal(test.Commands[0].Type, "kernel")
		assert.Equal(test.Commands[0].Input.Line, "boot")
		assert.Equal(test.Commands[1].Type, "user")
		assert.Equal(test.Commands[1].Input.Line, "s")
		assert.Equal(test.Commands[2].Type, "user")
		assert.Equal(test.Commands[2].Input.Line, "/testbin/waiter")
	}

	/*
		assert.Equal(test.Status, "monitor")
		assert.True(strings.HasPrefix(test.ShutdownMessage, "no progress"))
	*/
	assert.True(test.SimTime < 4.0)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}
