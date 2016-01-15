package test161

import (
	"github.com/stretchr/testify/assert"
	"reflect"
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
	assert.True(reflect.DeepEqual(test.Tags, []string{"testing", "test161"}))
	assert.True(reflect.DeepEqual(test.Depends, []string{"boot", "shell"}))

}

func TestConfDefaults(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("")
	assert.Nil(err)
	test.Sys161.Random = 0
	assert.Nil(test.MergeConf(CONF_DEFAULTS))

	assert.Equal(&CONF_DEFAULTS, test)
}

func TestConfOverrides(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString(`---
sys161:
  cpus: 1
  ram: 2M
  disk1:
    enabled: false
    bytes: 4M
    rpm: 14400
    nodoom: false
  disk2:
    enabled: true
    bytes: 6M
    rpm: 28800
    nodoom: true
stat:
  resolution: 0.0001
  window: 100
monitor:
  enabled: false
  window: 20
  kernel:
    enablemin: false
    min: 0.1
    max: 0.8
  user:
    enablemin: false
    min: 0.2
    max: 0.9
  progresstimeout: 20.0
misc:
  commandretries: 10
  prompttimeout: 100.0
  charactertimeout: 10
  tempdir: "/blah/"
---
`)
	assert.Nil(err)
	test.Sys161.Random = 0

	overrides := Test{
		Sys161: Sys161Conf{
			CPUs: 1,
			RAM:  "2M",
			Disk1: DiskConf{
				Enabled: "false",
				Bytes:   "4M",
				RPM:     14400,
				NoDoom:  "false",
			},
			Disk2: DiskConf{
				Enabled: "true",
				Bytes:   "6M",
				RPM:     28800,
				NoDoom:  "true",
			},
		},
		Stat: StatConf{
			Resolution: 0.0001,
			Window:     100,
		},
		Monitor: MonitorConf{
			Enabled: "false",
			Window:  20,
			Kernel: Limits{
				EnableMin: "false",
				Min:       0.1,
				Max:       0.8,
			},
			User: Limits{
				EnableMin: "false",
				Min:       0.2,
				Max:       0.9,
			},
			ProgressTimeout: 20.0,
		},
		Misc: MiscConf{
			CommandRetries:   10,
			PromptTimeout:    100.0,
			CharacterTimeout: 10,
			TempDir:          "/blah/",
		},
	}
	assert.Equal(&overrides, test)
	assert.Nil(test.MergeConf(CONF_DEFAULTS))
	assert.Equal(&overrides, test)
}
