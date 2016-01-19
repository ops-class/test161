package test161

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

type AbrevTest struct {
	Name    string
	Command string
	Deps    []string
}

func groupFromSlice(t *testing.T, tests []AbrevTest) (*TestGroup, error) {
	tg := EmptyGroup()
	tg.Config.UseDeps = true

	for _, loctest := range tests {
		test, err := TestFromString(loctest.Command)
		assert.Nil(t, err)
		if err != nil {
			return nil, err
		}
		test.Name = loctest.Name
		for _, dep := range loctest.Deps {
			test.Depends = append(test.Depends, dep)
		}
		tg.Tests = append(tg.Tests, test)
	}
	return tg, nil
}

func TestGroupDepNoCycle(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	var testStrings = []AbrevTest{
		AbrevTest{"boot", "q", []string{}},
		AbrevTest{"shell", "$ /bin/true", []string{"boot"}},
		AbrevTest{"shll", "$ /testbin/shll", []string{"boot"}},
		AbrevTest{"badcall", "$ /testbin/badcall a", []string{"shell"}},
		AbrevTest{"badcall2", "$ /testbin/badcall b", []string{"badcall", "shell"}},
		AbrevTest{"badcall3", "$ /testbin/badcall c", []string{"badcall"}},
		AbrevTest{"randcall", "$ /testbin/randcall", []string{"shell"}},
		AbrevTest{"forktest", "$ forktest", []string{"shell", "badcall2"}},
		AbrevTest{"bigfork", "$ bigfork", []string{"forktest"}},
	}

	tg, err := groupFromSlice(t, testStrings)
	assert.Nil(err)
	assert.NotNil(tg)

	assert.Equal(len(testStrings), len(tg.Tests))

	sorted, err := tg.VerifyDependencies()
	assert.Nil(err)
	if err == nil {
		t.Log(sorted)
	}
	assert.True(tg.CanRun())
}

func TestGroupDepCycle(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	// There is a cycle forktest -> badcall2 -> bigfork -> forktest
	var testStrings = []AbrevTest{
		AbrevTest{"boot", "q", []string{}},
		AbrevTest{"shell", "$ /bin/true", []string{"boot"}},
		AbrevTest{"shll", "$ /testbin/shll", []string{"boot"}},
		AbrevTest{"badcall", "$ /testbin/badcall a", []string{"shell"}},
		AbrevTest{"badcall2", "$ /testbin/badcall b", []string{"badcall", "shell", "bigfork"}},
		AbrevTest{"badcall3", "$ /testbin/badcall c", []string{"badcall"}},
		AbrevTest{"randcall", "$ /testbin/randcall", []string{"shell"}},
		AbrevTest{"forktest", "$ forktest", []string{"shell", "badcall2"}},
		AbrevTest{"bigfork", "$ bigfork", []string{"forktest"}},
	}

	tg, err := groupFromSlice(t, testStrings)
	assert.Nil(err)

	assert.Equal(len(testStrings), len(tg.Tests))

	sorted, err := tg.VerifyDependencies()
	assert.NotNil(err)
	if err == nil {
		t.Log(sorted)
	}

	assert.False(tg.CanRun())
}

func TestGroupCannotRun(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	// Test missing dependencies
	var testStrings = []AbrevTest{
		AbrevTest{"shell", "$ /bin/true", []string{"boot"}},
		AbrevTest{"randcall", "$ /testbin/randcall", []string{"shell"}},
		AbrevTest{"forktest", "$ forktest", []string{"shell", "badcall2"}},
		AbrevTest{"bigfork", "$ bigfork", []string{"forktest"}},
	}

	tg, err := groupFromSlice(t, testStrings)
	assert.Nil(err)
	assert.NotNil(tg)
	assert.False(tg.CanRun())

	// Test a simple cycle
	testStrings = []AbrevTest{
		AbrevTest{"boot", "q", []string{}},
		AbrevTest{"shell", "$ /bin/true", []string{"boot", "randcall"}},
		AbrevTest{"randcall", "$ /testbin/randcall", []string{"shell, boot"}},
	}

	tg, err = groupFromSlice(t, testStrings)
	assert.Nil(err)
	assert.NotNil(tg)
	assert.False(tg.CanRun())

	// Test an empty group
	tg = EmptyGroup()
	assert.False(tg.CanRun())

	// Test (erroneous) duplicate names where we're using dependencies
	testStrings = []AbrevTest{
		AbrevTest{"boot", "q", []string{}},
		AbrevTest{"boot", "$ /bin/true", []string{""}},
		AbrevTest{"randcall", "$ /testbin/randcall", []string{""}},
	}

	tg, err = groupFromSlice(t, testStrings)
	assert.Nil(err)
	assert.NotNil(tg)
	assert.False(tg.CanRun())

	// Test an unnamed group - not good for dependencies
	testStrings = []AbrevTest{
		AbrevTest{"", "q", []string{}},
		AbrevTest{"boot", "q", []string{""}},
		AbrevTest{"randcall", "$ /testbin/randcall", []string{""}},
	}

	tg, err = groupFromSlice(t, testStrings)
	assert.Nil(err)
	assert.NotNil(tg)
	assert.False(tg.CanRun())
}

func TestGroupFromDir(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	config := GroupConfig{"asst1", "./fixtures", true, "./fixtures/tests", []string{"common", "asst1"}, nil}
	tg, err := GroupFromConfig(config)
	assert.Nil(err)

	assert.True(tg.Config.UseDeps)
	assert.True(tg.CanRun())
	assert.Equal(2, len(tg.Tests))

	t.Log(tg.OutputJSON())
	t.Log(tg.OutputString())

	// This is should be the same as above.  A nil tags slice will
	// ignore tags
	config = GroupConfig{"asst1", "./fixtures", true, "./fixtures/tests", nil, nil}
	tg, err = GroupFromConfig(config)
	assert.Nil(err)

	assert.True(tg.Config.UseDeps)
	assert.True(tg.CanRun())
	assert.Equal(2, len(tg.Tests))

	t.Log(tg.OutputJSON())
	t.Log(tg.OutputString())

	config = GroupConfig{"asst1", "./fixtures", true, "./fixtures/tests", []string{"asst1"}, nil}
	tg, err = GroupFromConfig(config)
	assert.Nil(err)
	assert.Equal(1, len(tg.Tests))

	// This can't run b/c asst1 relies on boot, which is only tagged as 'common'
	assert.True(tg.Config.UseDeps)
	assert.False(tg.CanRun())

	t.Log(tg.OutputJSON())
	t.Log(tg.OutputString())

	config = GroupConfig{"asst1", "./fixtures", false, "./fixtures/tests", []string{"asst1"}, nil}
	tg, err = GroupFromConfig(config)
	assert.Nil(err)
	assert.Equal(1, len(tg.Tests))

	// This can run b/c we're ignoring dependencies
	assert.False(tg.Config.UseDeps)
	assert.True(tg.CanRun())

	t.Log(tg.OutputJSON())
	t.Log(tg.OutputString())

}
