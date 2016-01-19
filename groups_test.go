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
}

func TestGroupFromDir(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	config := GroupConfig{"asst1", "./fixtures", true, "./fixtures/tests", nil}
	tg, err := GroupFromConfig(config)
	assert.Nil(err)

	assert.True(tg.Config.UseDeps)
	assert.True(tg.CanRun())

	t.Log(tg.OutputJSON())
	t.Log(tg.OutputString())
}
