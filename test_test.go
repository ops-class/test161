package test161

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	rand.Seed(time.Now().UTC().UnixNano())
	os.Exit(m.Run())
}

func TestLoad(t *testing.T) {
	assert := assert.New(t)

	test, err := LoadTest("./fixtures/tests/boot.yml")
	assert.Nil(err)
	assert.Equal(test.Name, "boot")
	assert.NotNil(test.Description)
	assert.Equal(reflect.DeepEqual(test.Tags, []string{"basic", "setup"}), true)
	assert.Nil(test.Depends)
	assert.Equal(test.Conf.CPUs, (uint)(1))
	assert.Equal(test.Conf.RAM, "16777216")
	assert.Equal(test.Conf.Disk1.Sectors, "65536")
	assert.Equal(test.Conf.Disk1.RPM, (uint)(7200))

	test, err = LoadTest("./fixtures/tests/shell.yml")
	assert.Nil(err)
	assert.Equal(test.Name, "shell")
	assert.Equal(test.Description, "")
	assert.Nil(test.Tags)
	assert.Equal(reflect.DeepEqual(test.Depends, []string{"boot"}), true)
	assert.Equal(test.Conf.CPUs, (uint)(4))
	assert.Equal(test.Conf.RAM, "1048576")

	test, err = LoadTest("./fixtures/tests/parallelvm.yml")
	assert.Nil(err)
	assert.Equal(test.Name, "parallelvm")
	assert.Equal(test.Description, "")
	assert.Nil(test.Tags)
	assert.Equal(reflect.DeepEqual(test.Depends, []string{"shell"}), true)
	assert.Equal(test.Conf.CPUs, (uint)(4))
	assert.Equal(test.Conf.RAM, "2097152")
	assert.Equal(test.Conf.Disk2.Sectors, "10240")
	assert.Equal(test.Conf.Disk2.RPM, (uint)(14400))
}

func TestPrintConf(t *testing.T) {
	assert := assert.New(t)

	for _, test := range []string{"boot", "shell", "parallelvm"} {
		test, err := LoadTest(fmt.Sprintf("./fixtures/tests/%v.yml", test))
		assert.Nil(err)
		conf, err := test.PrintConf()
		assert.Nil(err)
		assert.NotNil(conf)
		t.Log(conf)
	}
}

func TestRunBoot(t *testing.T) {
	assert := assert.New(t)
	test, err := LoadTest("./fixtures/tests/tt1.yml")
	assert.Nil(err)
	err = test.Run("./fixtures/sol0/", "")
	assert.Nil(err)
	t.Log(test.OutputString())
}

func TestRunShell(t *testing.T) {
	assert := assert.New(t)

	test, err := LoadTest("./fixtures/tests/shell.yml")
	assert.Nil(err)

	err = test.Run("./fixtures/sol2/", "")
	assert.Nil(err)

	assert.Equal(test.Output.Status, "shutdown")

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestShutdownKernelPanic(t *testing.T) {
	assert := assert.New(t)

	test, err := LoadTest("./fixtures/tests/panic.yml")
	assert.Nil(err)

	err = test.Run("./fixtures/sol0/", "")
	assert.Nil(err)

	assert.Equal(test.Output.Status, "crash")

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestMonitorKernelDeadlock(t *testing.T) {
	assert := assert.New(t)

	test, err := LoadTest("./fixtures/tests/dl.yml")
	assert.Nil(err)

	test.MonitorConf.MinKernel = 0.0
	test.Timeout = 4

	err = test.Run("./fixtures/sol2/", "")
	assert.Nil(err)

	assert.Equal(test.Output.Status, "timeout")

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())

	test, err = LoadTest("./fixtures/tests/dl.yml")
	assert.Nil(err)
	test.Timeout = 4

	err = test.Run("./fixtures/sol2/", "")
	assert.Nil(err)

	assert.Equal(test.Output.Status, "monitor")
	assert.True(test.Output.RunTime < 4.0)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}

func TestMonitorKernelLivelock(t *testing.T) {
	assert := assert.New(t)

	test, err := LoadTest("./fixtures/tests/ll16.yml")
	assert.Nil(err)

	test.MonitorConf.MinKernel = 0.0
	test.Timeout = 4

	err = test.Run("./fixtures/sol2/", "")
	assert.Nil(err)

	assert.Equal(test.Output.Status, "timeout")

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())

	test, err = LoadTest("./fixtures/tests/ll16.yml")
	assert.Nil(err)
	test.Timeout = 8

	err = test.Run("./fixtures/sol2/", "")
	assert.Nil(err)

	assert.Equal(test.Output.Status, "monitor")
	assert.True(test.Output.RunTime < 8.0)

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}
