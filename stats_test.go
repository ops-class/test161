package test161

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestStatsKernelDeadlock(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("dl")
	assert.Nil(err)
	test.Monitor.ProgressTimeout = 8.0

	err = test.Run("./fixtures/")
	assert.Nil(err)

	assert.Equal(test.Status, "monitor")
	assert.True(test.SimTime < 8.0)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestStatsKernelLivelock(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("ll16")
	assert.Nil(err)
	test.Monitor.ProgressTimeout = 8.0

	err = test.Run("./fixtures/")
	assert.Nil(err)

	assert.Equal(test.Status, "monitor")
	assert.True(test.SimTime < 8.0)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestStatsUserDeadlock(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("$ /testbin/waiter")
	assert.Nil(err)

	test.Monitor.Kernel.Min = 0.0
	test.Misc.PromptTimeout = 8.0

	err = test.Run("./fixtures/")
	assert.Nil(err)

	assert.Equal(test.Status, "monitor")
	assert.True(test.SimTime < 8.0)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestStatsKernelProgress(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("ll16")
	assert.Nil(err)

	test.Monitor.Kernel.Min = 0.0
	test.Monitor.Kernel.Max = 1.0
	test.Monitor.ProgressTimeout = 8.0
	test.Misc.PromptTimeout = 8.0

	err = test.Run("./fixtures/")
	assert.Nil(err)

	assert.Equal(test.Status, "monitor")
	assert.True(test.SimTime < 10.0)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestStatsUserProgress(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("$ /testbin/waiter")
	assert.Nil(err)

	test.Monitor.Kernel.Min = 0.0
	test.Monitor.User.Min = 0.0
	test.Monitor.ProgressTimeout = 2.0
	test.Misc.PromptTimeout = 60.0

	err = test.Run("./fixtures/")
	assert.Nil(err)

	assert.Equal(test.Status, "monitor")
	assert.True(test.SimTime < 10.0)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}
