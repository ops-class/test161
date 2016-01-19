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
q
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

	test, err := TestFromString("q")
	assert.Nil(err)
	test.Sys161.Random = 0
	assert.Nil(test.MergeConf(CONF_DEFAULTS))
	assert.True(test.confEqual(&CONF_DEFAULTS))
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
  retrycharacters: false
  killonexit: true
---
q
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
			RetryCharacters:  "false",
			KillOnExit:       "true",
		},
	}
	assert.True(test.confEqual(&overrides))
	assert.Nil(test.MergeConf(CONF_DEFAULTS))
	assert.True(test.confEqual(&overrides))
}

func TestConfCommandInit(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromString("q")
	assert.Nil(err)
	assert.Equal(len(test.Commands), 2)
	if len(test.Commands) == 2 {
		assert.Equal(test.Commands[0].Type, "kernel")
		assert.Equal(test.Commands[0].Input.Line, "boot")
		assert.Equal(test.Commands[1].Type, "kernel")
		assert.Equal(test.Commands[1].Input.Line, "q")
	}

	test, err = TestFromString("$ /bin/true")
	assert.Nil(err)
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

	test, err = TestFromString("$ /bin/true\n$ exit")
	assert.Nil(err)
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

	test, err = TestFromString("panic")
	assert.Nil(err)
	assert.Equal(len(test.Commands), 3)
	if len(test.Commands) == 3 {
		assert.Equal(test.Commands[0].Type, "kernel")
		assert.Equal(test.Commands[0].Input.Line, "boot")
		assert.Equal(test.Commands[1].Type, "kernel")
		assert.Equal(test.Commands[1].Input.Line, "panic")
		assert.Equal(test.Commands[2].Type, "kernel")
		assert.Equal(test.Commands[2].Input.Line, "q")
	}
}

func TestConfBroken(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	// Broken front matter
	_, err := TestFromString(`---
unused
---
q`)
	assert.NotNil(err)

	// Double exit
	_, err = TestFromString("q\nq")
	assert.NotNil(err)

	// Empty command
	_, err = TestFromString(" \n ")
	assert.NotNil(err)
}

func TestConfSplitPrefix(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	prefix, commandLine := splitPrefix("$ simple")
	assert.Equal(prefix, "$")
	assert.Equal(commandLine, "simple")

	prefix, commandLine = splitPrefix("$  whitespace ")
	assert.Equal(prefix, "$")
	assert.Equal(commandLine, "whitespace")

	prefix, commandLine = splitPrefix("^  another ")
	assert.Equal(prefix, "^")
	assert.Equal(commandLine, "another")

	prefix, commandLine = splitPrefix("p not_a_prefix  ")
	assert.Equal(prefix, "")
	assert.Equal(commandLine, "p not_a_prefix")
}

func TestConfCheckCommandConf(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	// Check empty
	test, err := confFromString("q")
	assert.Nil(err)
	err = test.checkCommandConf()
	assert.Nil(err)

	// Check valid
	test, err = confFromString(`---
commandconf:
  - prefix: "!"
    prompt: TEST
    start: $ test
    end: test
---
q`)
	assert.Nil(err)
	err = test.checkCommandConf()
	assert.Nil(err)

	// Check valid, multiple
	test, err = confFromString(`---
commandconf:
  - prefix: "^"
    prompt: TEST
    start: "! test"
    end: test
  - prefix: "!"
    prompt: TEST
    start: $ test
    end: test
---
q`)
	assert.Nil(err)
	err = test.checkCommandConf()
	assert.Nil(err)

	// Check invalid: missing start
	test, err = confFromString(`---
commandconf:
  - prefix: "!"
    prompt: TEST
    end: test
---
q`)
	assert.Nil(err)
	err = test.checkCommandConf()
	assert.NotNil(err)

	// Check invalid: multiple character prefix
	test, err = confFromString(`---
commandconf:
  - prefix: "!!"
    prompt: TEST
    start: $ test
    end: test
---
q`)
	assert.Nil(err)
	err = test.checkCommandConf()
	assert.NotNil(err)

	// Check invalid: bad prefix
	test, err = confFromString(`---
commandconf:
  - prefix: "."
    prompt: TEST
    start: $ test
    end: test
---
q`)
	assert.Nil(err)
	err = test.checkCommandConf()
	assert.NotNil(err)

	// Check invalid: collides with shell
	test, err = confFromString(`---
commandconf:
  - prefix: $ 
    prompt: TEST
    start: test
    end: test
---
q`)
	assert.Nil(err)
	err = test.checkCommandConf()
	assert.NotNil(err)

	// Check invalid: duplicate
	test, err = confFromString(`---
commandconf:
  - prefix: "!"
    prompt: TEST
    start: "$ test"
    end: test
  - prefix: "!"
    prompt: TEST
    start: $ test
    end: test
---
q`)
	assert.Nil(err)
	err = test.checkCommandConf()
	assert.NotNil(err)

	// Check invalid: start with own prefix
	test, err = confFromString(`---
commandconf:
  - prefix: "!"
    prompt: TEST
    start: "! test"
    end: test
---
q`)
	assert.Nil(err)
	err = test.checkCommandConf()
	assert.NotNil(err)

	// Check invalid: end with prefix
	test, err = confFromString(`---
commandconf:
  - prefix: "!"
    prompt: TEST
    start: $ test
    end: "^ test"
---
q`)
	assert.Nil(err)
	err = test.checkCommandConf()
	assert.NotNil(err)

	// Check invalid: start with unknown prefix
	test, err = confFromString(`---
commandconf:
  - prefix: "%"
    prompt: TEST
    start: $ test
    end: test
  - prefix: ^
    prompt: TEST
    start: "! blah"
    end: missing
---
q`)
	assert.Nil(err)
	err = test.checkCommandConf()
	assert.NotNil(err)

}

func TestConfFromFile(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	test, err := TestFromFile("./fixtures/tests/shell.yaml")
	assert.Nil(err)
	assert.NotNil(test)
	if test != nil {
		assert.Equal("shell", test.Name)
		assert.Equal(1, len(test.Depends))
		if len(test.Depends) == 1 {
			assert.Equal("boot", test.Depends[0])
		}
		assert.Equal(float32(.01), test.Stat.Resolution)
		assert.Equal(uint(100), test.Stat.Window)
		assert.Equal(float32(30.0), test.Misc.PromptTimeout)
	}

	test, err = TestFromFile("./fixtures/tests/does_not_exist.yaml")
	assert.NotNil(err)
	assert.Nil(test)
}
