package test161

import (
	"github.com/stretchr/testify/assert"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestConfMetadata(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString(`---
name: test
description: <
  Testing metadata.
tags: ["testing", "test161"]
depends:
- boot
- shell
---
`)
	assert.Nil(err)

	assert.Equal(test.Name, "test")
	assert.NotEqual(test.Description, "")
	assert.Equal(reflect.DeepEqual(test.Tags, []string{"testing", "test161"}), true)
	assert.Equal(reflect.DeepEqual(test.Depends, []string{"boot", "shell"}), true)

}

func TestConfDefaults(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("")
	assert.Nil(err)

	assert.Equal(test.Conf.CPUs, uint(8))
	assert.Equal(test.Conf.RAM, strconv.Itoa(1024*1024))
	assert.Equal(test.Conf.Disk1.Sectors, strconv.Itoa(8000))
	assert.Equal(test.Conf.Disk1.RPM, uint(7200))
	assert.Equal(test.Conf.Disk1.NoDoom, "true")
	assert.Equal(test.Conf.Disk1.File, "LHD0.img")
	assert.True(strings.HasPrefix(test.Conf.Random, "seed="))

	assert.Equal(test.MonitorConf.Enabled, "true")
	assert.Equal(test.MonitorConf.Window, float32(2.0))
	assert.Equal(test.MonitorConf.Resolution, uint(100))
	assert.Equal(test.MonitorConf.Timeouts.Prompt, uint(5*60))
	assert.Equal(test.MonitorConf.Timeouts.Progress, uint(60))
	assert.Equal(test.MonitorConf.Kernel.Min, 0.001)
	assert.Equal(test.MonitorConf.Kernel.Max, 0.99)
	assert.Equal(test.MonitorConf.User.Min, 0.0001)
	assert.Equal(test.MonitorConf.User.Max, 1.0)

	test, err = TestFromString(`---
monitor:
  allstats: true
---
`)
	assert.Nil(err)

	assert.Equal(test.MonitorConf.Resolution, uint(50000))
}

func TestConfOverrides(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString(`---
name: override
conf:
  cpus: 1
  ram: 2M
  disk1:
    sectors: 10240
    rpm: 14400
    nodoom: false
  disk2:
    sectors: 1024
  random: seed=1024
monitor:
  enabled: false
  allstats: true
  window: 2.5
  resolution: 2000
  kernel:
    min: 0.1
    max: 0.8
  user:
    min: 0.2
    max: 0.9
  timeouts:
    prompt: 60
    progress: 30
---
`)

	assert.Nil(err)
	assert.Equal(test.Name, "override")

	assert.Equal(test.Conf.CPUs, uint(1))
	assert.Equal(test.Conf.RAM, strconv.Itoa(2*1024*1024))
	assert.Equal(test.Conf.Disk1.Sectors, strconv.Itoa(10240))
	assert.Equal(test.Conf.Disk1.RPM, uint(14400))
	assert.Equal(test.Conf.Disk1.NoDoom, "false")
	assert.Equal(test.Conf.Disk2.Sectors, strconv.Itoa(1024))
	assert.Equal(test.Conf.Disk2.RPM, uint(7200))
	assert.Equal(test.Conf.Disk2.NoDoom, "false")
	assert.Equal(test.Conf.Disk2.File, "LHD1.img")
	assert.Equal(test.Conf.Random, "seed=1024")

	assert.Equal(test.MonitorConf.Enabled, "false")
	assert.Equal(test.MonitorConf.AllStats, "true")
	assert.Equal(test.MonitorConf.Window, float32(2.5))
	assert.Equal(test.MonitorConf.Resolution, uint(2000))
	assert.Equal(test.MonitorConf.Timeouts.Prompt, uint(60))
	assert.Equal(test.MonitorConf.Timeouts.Progress, uint(30))
	assert.Equal(test.MonitorConf.Kernel.Min, 0.1)
	assert.Equal(test.MonitorConf.Kernel.Max, 0.8)
	assert.Equal(test.MonitorConf.User.Min, 0.2)
	assert.Equal(test.MonitorConf.User.Max, 0.9)
}

func TestConfPrintConf(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("")
	assert.Nil(err)

	conf, err := test.PrintConf()
	assert.Nil(err)
	assert.NotNil(conf)
	t.Log(conf)
}
