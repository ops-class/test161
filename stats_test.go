package test161

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestStatsKernelDeadlock(t *testing.T) {
	assert := assert.New(t)

	test, err := TestFromString("dl")
	assert.Nil(err)

	test.MonitorConf.Kernel.Min = 0.0
	test.MonitorConf.Timeouts.Prompt = 4

	err = test.Run("./fixtures/", "")
	assert.Nil(err)

	assert.Equal(test.Status, "timeout")

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())

	test, err = TestFromString("dl")
	assert.Nil(err)
	test.MonitorConf.Timeouts.Prompt = 4

	err = test.Run("./fixtures/", "")
	assert.Nil(err)

	assert.Equal(test.Status, "monitor")
	assert.True(test.RunTime < 4.0)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestStatsKernelLivelock(t *testing.T) {
	assert := assert.New(t)

	test, err := TestFromString("ll16")
	assert.Nil(err)

	test.MonitorConf.Kernel.Max = 1.0
	test.MonitorConf.Timeouts.Prompt = 4

	err = test.Run("./fixtures/", "")
	assert.Nil(err)

	assert.Equal(test.Status, "timeout")

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())

	test, err = TestFromString("ll16")
	assert.Nil(err)
	test.MonitorConf.Timeouts.Prompt = 4
	test.MonitorConf.Intervals = 5

	err = test.Run("./fixtures/", "")
	assert.Nil(err)

	assert.Equal(test.Status, "monitor")
	assert.True(test.RunTime < 4.0)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestStatsUserDeadlock(t *testing.T) {
	assert := assert.New(t)

	test, err := TestFromString("$ /testbin/waiter")
	assert.Nil(err)

	test.MonitorConf.User.Min = 0.0
	test.MonitorConf.Kernel.Min = 0.0
	test.MonitorConf.Timeouts.Prompt = 4

	err = test.Run("./fixtures/", "")
	assert.Nil(err)

	assert.Equal(test.Status, "timeout")

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())

	test, err = TestFromString("$ /testbin/waiter")
	assert.Nil(err)

	test.MonitorConf.Kernel.Min = 0.0
	test.MonitorConf.Timeouts.Prompt = 4
	test.MonitorConf.Intervals = 5

	err = test.Run("./fixtures/", "")
	assert.Nil(err)

	assert.Equal(test.Status, "monitor")
	assert.True(test.RunTime < 4.0)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}
