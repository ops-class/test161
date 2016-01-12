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
	test.MonitorConf.Timeouts.Progress = 8

	err = test.Run("./fixtures/", "")
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
	test.MonitorConf.Timeouts.Progress = 8

	err = test.Run("./fixtures/", "")
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

	test.MonitorConf.Kernel.Min = 0.0
	test.MonitorConf.Timeouts.Prompt = 8

	err = test.Run("./fixtures/", "")
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

	test.MonitorConf.Kernel.Min = 0.0
	test.MonitorConf.Kernel.Max = 1.0
	test.MonitorConf.Timeouts.Progress = 2
	test.MonitorConf.Timeouts.Prompt = 60

	err = test.Run("./fixtures/", "")
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

	test.MonitorConf.Kernel.Min = 0.0
	test.MonitorConf.User.Min = 0.0
	test.MonitorConf.Timeouts.Progress = 2
	test.MonitorConf.Timeouts.Prompt = 60

	err = test.Run("./fixtures/", "")
	assert.Nil(err)

	assert.Equal(test.Status, "monitor")
	assert.True(test.SimTime < 10.0)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}
