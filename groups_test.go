package test161

import (
	//"github.com/ops-class/test161/graph"
	"github.com/stretchr/testify/assert"
	"path/filepath"
	"sort"
	"testing"
)

const TEST_DIR string = "fixtures/tests/nocycle"
const CYCLE_DIR string = "fixtures/tests/cycle"

func testsToSortedSlice(tests []*Test) []string {
	res := make([]string, len(tests))
	for i, t := range tests {
		res[i] = t.DependencyID
	}
	sort.Strings(res)
	return res
}

func TestTestMapLoad(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	tm, errs := newTestMap(TEST_DIR)
	assert.NotNil(tm)
	assert.Equal(0, len(errs))

	expected := []string{
		"boot.t",
		"panics/panic.t",
		"panics/deppanic.t",
		"threads/tt1.t",
		"threads/tt2.t",
		"threads/tt3.t",
		"sync/all.t",
		"sync/fail.t",
		"sync/multi.t",
		"sync/sy1.t",
		"sync/sy2.t",
		"sync/sy3.t",
		"sync/sy4.t",
		"sync/sy5.t",
		"sync/semu1.t",
	}

	assert.Equal(len(expected), len(tm.Tests))

	for _, id := range expected {
		_, ok := tm.Tests[id]
		assert.True(ok)
	}

	expected = []string{
		"boot", "threads", "sync",
		"sem", "locks", "rwlock", "cv",
	}

	assert.Equal(len(expected), len(tm.Tags))

	for _, id := range expected {
		_, ok := tm.Tags[id]
		assert.True(ok)
	}
}

func TestTestMapGlobs(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	abs, err := filepath.Abs(TEST_DIR)
	assert.Nil(err)

	tm, errs := newTestMap(TEST_DIR)
	assert.NotNil(tm)
	assert.Equal(0, len(errs))

	// Glob
	tests, err := tm.testsFromGlob("**/sy*.t", abs)
	expected := []string{
		"sync/sy1.t",
		"sync/sy2.t",
		"sync/sy3.t",
		"sync/sy4.t",
		"sync/sy5.t",
	}

	assert.Nil(err)
	assert.Equal(len(expected), len(tests))

	actual := testsToSortedSlice(tests)
	assert.Equal(expected, actual)

	// Single test
	single := "threads/tt2.t"
	tests, err = tm.testsFromGlob(single, abs)
	assert.Nil(err)
	assert.Equal(1, len(tests))
	if len(tests) == 1 {
		assert.Equal(single, tests[0].DependencyID)
	}

	// Empty
	tests, err = tm.testsFromGlob("foo/bar*.t", abs)
	assert.NotNil(err)
	assert.Equal(0, len(tests))

}

func TestTestMapTags(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	tm, errs := newTestMap(TEST_DIR)
	assert.NotNil(tm)
	assert.Equal(0, len(errs))

	expected := []string{
		"threads/tt1.t",
		"threads/tt2.t",
		"threads/tt3.t",
	}
	tests, ok := tm.Tags["threads"]
	assert.True(ok)
	assert.Equal(len(expected), len(tests))

	actual := testsToSortedSlice(tests)
	sort.Strings(actual)
	assert.Equal(expected, actual)

	expected = []string{
		"sync/sy3.t",
		"sync/sy4.t",
	}
	tests, ok = tm.Tags["cv"]
	assert.True(ok)
	assert.Equal(len(expected), len(tests))

	actual = testsToSortedSlice(tests)
	sort.Strings(actual)
	assert.Equal(expected, actual)

}

var DEP_MAP = map[string][]string{
	"boot.t":            []string{},
	"threads/tt1.t":     []string{"boot.t"},
	"threads/tt2.t":     []string{"boot.t"},
	"threads/tt3.t":     []string{"boot.t"},
	"sync/semu1.t":      []string{"threads/tt1.t", "threads/tt2.t", "threads/tt2.t"},
	"sync/sy1.t":        []string{"threads/tt1.t", "threads/tt2.t", "threads/tt2.t"},
	"sync/sy2.t":        []string{"threads/tt1.t", "threads/tt2.t", "threads/tt2.t"},
	"sync/sy3.t":        []string{"threads/tt1.t", "threads/tt2.t", "threads/tt2.t", "sync/sy2.t"},
	"sync/sy4.t":        []string{"threads/tt1.t", "threads/tt2.t", "threads/tt2.t", "sync/sy2.t", "sync/sy3.t"},
	"sync/sy5.t":        []string{"threads/tt1.t", "threads/tt2.t", "threads/tt2.t"},
	"sync/multi.t":      []string{"threads/tt1.t", "threads/tt2.t", "threads/tt2.t"},
	"sync/all.t":        []string{"threads/tt1.t", "threads/tt2.t", "threads/tt2.t"},
	"sync/fail.t":       []string{"threads/tt1.t", "threads/tt2.t", "threads/tt2.t"},
	"panics/panic.t":    []string{"boot.t"},
	"panics/deppanic.t": []string{"panics/panic.t"},
}

func TestTestMapDependencies(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	tm, errs := newTestMap(TEST_DIR)
	assert.NotNil(tm)
	assert.Equal(0, len(errs))

	errs = tm.expandAllDeps()
	assert.Equal(0, len(errs))
	if len(errs) > 0 {
		t.Log(errs)
	}

	// Now, test the dependencies by hand.  We have a mix of
	// glob and tag deps in the test directory

	assert.Equal(len(DEP_MAP), len(tm.Tests))

	for k, v := range DEP_MAP {
		test, ok := tm.Tests[k]
		assert.True(ok)
		if ok {
			assert.Equal(len(v), len(test.ExpandedDeps))
			for _, id := range v {
				dep, ok := test.ExpandedDeps[id]
				assert.True(ok)
				assert.Equal(id, dep.DependencyID)
			}
		}
	}
}

func TestDependencyGraph(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	tm, errs := newTestMap(TEST_DIR)
	assert.NotNil(tm)
	assert.Equal(0, len(errs))

	g, errs := tm.dependencyGraph()
	assert.Equal(0, len(errs))
	if len(errs) > 0 {
		t.Log(errs)
	}

	assert.NotNil(g)

	// Now, test the dependencies by hand.  We have a mix of
	// glob and tag deps in the test directory

	assert.Equal(len(DEP_MAP), len(g.NodeMap))

	for k, v := range DEP_MAP {
		node, ok := g.NodeMap[k]
		assert.True(ok)
		if ok {
			assert.Equal(len(v), len(node.EdgesOut))
			for _, id := range v {
				depNode, ok := node.EdgesOut[id]
				assert.True(ok)
				assert.Equal(id, depNode.Name)
			}
		}
	}
}

func TestDependencyCycle(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	tm, errs := newTestMap(TEST_DIR)
	assert.NotNil(tm)
	assert.Equal(0, len(errs))

	g, errs := tm.dependencyGraph()
	assert.Equal(0, len(errs))
	if len(errs) > 0 {
		t.Log(errs)
	}

	assert.NotNil(g)
	_, err := g.TopSort()
	assert.Nil(err)

	tm, errs = newTestMap(CYCLE_DIR)
	assert.NotNil(tm)
	assert.Equal(0, len(errs))

	g, errs = tm.dependencyGraph()
	assert.Equal(0, len(errs))
	if len(errs) > 0 {
		t.Log(errs)
	}

	assert.NotNil(g)
	_, err = g.TopSort()
	assert.NotNil(err)
}

func TestGroupFromConfg(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	// Test config with dependencies
	config := &GroupConfig{
		Name:    "Test",
		UseDeps: true,
		Tests:   []string{"sync/sy1.t"},
		Env:     defaultEnv,
	}

	expected := []string{
		"boot.t", "threads/tt1.t", "threads/tt2.t",
		"threads/tt3.t", "sync/sy1.t",
	}

	tg, errs := GroupFromConfig(config)
	assert.Equal(0, len(errs))
	assert.NotNil(tg)

	assert.Equal(len(expected), len(tg.Tests))
	for _, id := range expected {
		test, ok := tg.Tests[id]
		assert.True(ok)
		if ok {
			assert.Equal(id, test.DependencyID)
		}
	}

	t.Log(tg)

	// Test same config without dependencies
	config.UseDeps = false
	tg, errs = GroupFromConfig(config)
	assert.Equal(0, len(errs))
	assert.NotNil(tg)
	assert.Equal(1, len(tg.Tests))
	id := config.Tests[0]
	test, ok := tg.Tests[id]
	assert.True(ok)
	if ok {
		assert.Equal(id, test.DependencyID)
	}

	t.Log(tg)
}

func TestGroupConfigInvalid(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	tests := []string{
		"threads/tt1.t",
		"threads/tt2.t",
		"thread/tt3.t",
	}

	// Test config with dependencies
	config := &GroupConfig{
		Name:    "Test",
		UseDeps: false,
		Tests:   tests,
		Env:     defaultEnv,
	}

	tg, errs := GroupFromConfig(config)
	assert.NotEqual(0, len(errs))
	t.Log(errs)
	assert.Nil(tg)
}
