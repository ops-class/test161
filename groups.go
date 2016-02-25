package test161

import (
	"errors"
	"fmt"
	"github.com/bmatcuk/doublestar"
	"github.com/ops-class/test161/graph"
	"path"
	"path/filepath"
	"strings"
)

// GroupConfig specifies how a group of tests should be created and run.
type GroupConfig struct {
	Name    string           `json:name`
	UseDeps bool             `json:"usedeps"`
	Tests   []string         `json:"tests"`
	Env     *TestEnvironment `json:"-" "bson:"-"`
}

// A group of tests to be run, which is the result of expanding a GroupConfig.
type TestGroup struct {
	Tests  map[string]*Test
	Config *GroupConfig
}

// EmptyGroup creates an empty TestGroup that can be used to add groups from
// strings.
func EmptyGroup() *TestGroup {
	tg := &TestGroup{}
	tg.Tests = make(map[string]*Test)
	return tg
}

// TagMap stores Tests indexed by id and maintains a map
// of tag -> tests for the test set.
type TagMap map[string][]*Test

// TestMap stores a mapping of test id -> test
type testMap struct {
	TestDir string
	Tests   map[string]*Test
	Tags    TagMap
}

// Result type for loading a test from a file
type testLoadResult struct {
	Test *Test
	Err  error
}

func newTestMap(testDir string) (*testMap, []error) {
	abs, err := filepath.Abs(testDir)
	if err != nil {
		return nil, []error{err}
	}
	abs = path.Clean(abs)
	tm := &testMap{abs, make(map[string]*Test), make(TagMap)}
	errs := tm.load()
	if len(errs) > 0 {
		return nil, errs
	}

	tm.buildTagMap()

	return tm, nil
}

// Helper function to get an id for a particular filename. We use the filename
// relative to the test directory.
func idFromFile(filename, testDir string) (string, error) {
	if temp, err := filepath.Abs(filename); err != nil {
		return "", err
	} else {
		return filepath.Rel(testDir, path.Clean(temp))
	}
}

// Creates a mapping of tag -> []test
func (tm *testMap) buildTagMap() {
	tm.Tags = make(TagMap)

	for _, test := range tm.Tests {
		for _, tag := range test.Tags {
			if _, ok := tm.Tags[tag]; !ok {
				tm.Tags[tag] = make([]*Test, 0)
			}
			tm.Tags[tag] = append(tm.Tags[tag], test)
		}
	}
}

// Helper function that just gets all test in the config test directory.
// It returns a mapping of test name (file name) to test.
func (tm *testMap) load() []error {
	errs := make([]error, 0)

	// Find all test files using globstar snytax
	// This is relative to our working directory
	files, err := doublestar.Glob(fmt.Sprintf("%v/**/*.t", tm.TestDir))
	if err != nil {
		errs = append(errs, err)
		return errs
	}

	resChan := make(chan testLoadResult)

	// Spawn a bunch of workers to load the tests
	for _, file := range files {
		go func(f string) {
			test, err := TestFromFile(f)
			if err == nil {
				test.DependencyID, err = idFromFile(f, tm.TestDir)
			}
			res := testLoadResult{test, err}
			resChan <- res
		}(file)
	}

	// Retrieve the results
	for i := 0; i < len(files); i++ {
		res := <-resChan
		if res.Err != nil {
			errs = append(errs, res.Err)
		} else {
			tm.Tests[res.Test.DependencyID] = res.Test
		}
	}

	return errs
}

// Get a slice of tests from a single search expression, which can be a glob
// or single file.
func (tm *testMap) testsFromGlob(search, startDir string) ([]*Test, error) {
	var glob string

	if strings.HasPrefix(search, "/") {
		// Relative to the test directory
		glob = path.Join(tm.TestDir, search)
	} else {
		// Relative to the path of the current file
		glob = path.Join(startDir, search)
	}

	// Test directories need to be self-contained. Clean up the
	// path and make that's where we're looking.
	glob = path.Clean(glob)

	if !strings.HasPrefix(glob, tm.TestDir) {
		return nil,
			errors.New(fmt.Sprintf("Cannot specify tests outside of testing directory: %v",
				glob))
	}

	// Get the files
	files, err := doublestar.Glob(glob)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, errors.New(fmt.Sprintf("Cannot find a file match for glob: %v'", glob))
	}

	// Finally, create a slice of tests corresponding to the search string
	tests := make([]*Test, 0)
	for _, file := range files {
		if id, err := idFromFile(file, tm.TestDir); err != nil {
			return nil, err
		} else {
			if test, ok := tm.Tests[id]; ok {
				tests = append(tests, test)
			} else {
				return nil,
					errors.New(fmt.Sprintf("Cannot find test: %v.  Is testMap initialized?", id))
			}
		}
	}
	return tests, nil
}

// Expand the dependencies for a single test
func (t *Test) expandTestDeps(tests *testMap, done chan error) {

	t.ExpandedDeps = make(map[string]*Test)

	for _, dep := range t.Depends {
		var deps []*Test = nil
		var ok bool = false
		var err error = nil

		if strings.HasSuffix(dep, ".t") {
			// it's a file/glob
			startDir := path.Dir(path.Join(tests.TestDir, t.DependencyID))
			if deps, err = tests.testsFromGlob(dep, startDir); err != nil {
				done <- err
				return
			} else if len(deps) == 0 {
				// The dependency doesn't exist at all
				done <- errors.New(fmt.Sprintf("No matches for dependency %v in test %v",
					dep, t.DependencyID))
				return
			}
		} else {
			// it's a tag, look it up in the tag map
			if deps, ok = tests.Tags[dep]; !ok {
				done <- errors.New(fmt.Sprintf("No matches for tag dependency '%v' in test '%v'",
					dep, t.DependencyID))
				return
			}
		}

		if deps != nil {
			for _, d := range deps {
				t.ExpandedDeps[d.DependencyID] = d
			}
		}
	}

	done <- nil
}

// Expand all tests' dependencies so we can create a dependency graph
func (tm *testMap) expandAllDeps() []error {
	resChan := make(chan error)
	errors := make([]error, 0)

	// Expand all test dependencies in parallel
	for _, t := range tm.Tests {
		go t.expandTestDeps(tm, resChan)
	}

	// Read the results
	for i, count := 0, len(tm.Tests); i < count; i++ {
		res := <-resChan
		if res != nil {
			errors = append(errors, res)
		}
	}

	return errors
}

// Keyer interface for the dependency graph
func (t *Test) Key() string {
	return t.DependencyID
}

func (tg *TestGroup) DependencyGraph() (*graph.Graph, error) {
	// Nodes
	nodes := make([]graph.Keyer, 0, len(tg.Tests))
	for _, t := range tg.Tests {
		nodes = append(nodes, t)
	}

	// Our graph
	g := graph.New(nodes)

	// Edges.  There is an edge from A->B if A depends on B.
	for _, test := range tg.Tests {
		for _, dep := range test.ExpandedDeps {
			if err := g.AddEdge(test, dep); err != nil {
				return nil, err
			}
		}
	}

	return g, nil
}

// DependencyGraph creates a dependency graph from the tests in testMap.
func (tm *testMap) dependencyGraph() (*graph.Graph, []error) {
	errs := tm.expandAllDeps()
	if len(errs) > 0 {
		return nil, errs
	}

	// Nodes
	nodes := make([]graph.Keyer, 0, len(tm.Tests))
	for _, t := range tm.Tests {
		nodes = append(nodes, t)
	}

	// Our graph
	g := graph.New(nodes)

	// Edges.  There is an edge from A->B if A depends on B.

	errs = make([]error, 0)
	for _, test := range tm.Tests {
		for _, dep := range test.ExpandedDeps {
			err := g.AddEdge(test, dep)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	return g, errs
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// The encapsulates the results of expanding a test expression, which can
// expand to multiple tests.  These get passed on the results channel between
// worker goroutines and their caller.
type expandRes struct {
	Err   error
	Tests []*Test
}

// Expand all the tests specified in a TestGroups' configuration.
func (tg *TestGroup) expandTests(tm *testMap) []error {
	resChan := make(chan *expandRes)

	// Spawn some workers to expand the tests
	for _, t := range tg.Config.Tests {
		go func(test string) {
			var tests []*Test = nil
			var err error = nil
			if strings.HasSuffix(test, ".t") {
				tests, err = tm.testsFromGlob(test, tg.Config.Env.TestDir)
			} else {
				if res, ok := tm.Tags[test]; ok {
					tests = res
				} else {
					err = errors.New(fmt.Sprintf("Cannot find tag: %v", test))
				}
			}
			resChan <- &expandRes{err, tests}
		}(t)
	}

	tg.Tests = make(map[string]*Test, 0)
	errs := make([]error, 0)

	// Get the results
	for i, count := 0, len(tg.Config.Tests); i < count; i++ {
		res := <-resChan
		if res.Err != nil {
			errs = append(errs, res.Err)
		} else {
			for _, test := range res.Tests {
				tg.Tests[test.DependencyID] = test
			}
		}
	}

	return errs
}

func getDepNodesHelper(node *graph.Node, tests []*Test) []*Test {
	// We have to do a type assertion to get a *Test from a Keyer
	v := node.Value
	var test *Test = v.(*Test)
	tests = append(tests, test)

	for _, node := range node.EdgesOut {
		tests = getDepNodesHelper(node, tests)
	}

	return tests
}

// If there's a cycle, this will loop forever, or at least until we run out of
// memory.
func getDepNodes(g *graph.Graph, id string, ch chan *expandRes) {

	// Just to be extra cautious
	if node, ok := g.NodeMap[id]; !ok {
		ch <- &expandRes{errors.New(fmt.Sprintf("Cannot find '%v' in dependency graph", id)), nil}
	} else {
		tests := getDepNodesHelper(node, make([]*Test, 0))
		ch <- &expandRes{nil, tests}
	}
}

// Create a TestGroup from a GroupConfig.  All test expressions are expanded
// and dependencies are added if UseDeps is set to true in the configuration.
func GroupFromConfig(config *GroupConfig) (*TestGroup, []error) {

	// Get all tests first
	tm, errs := newTestMap(config.Env.TestDir)
	if len(errs) > 0 {
		return nil, errs
	}

	tg := EmptyGroup()
	tg.Config = config

	// Convert to abs path since it's expected later
	var err error
	config.Env.TestDir, err = filepath.Abs(config.Env.TestDir)
	if err != nil {
		return nil, []error{err}
	}

	// Next, expand the tests provided as input
	errs = tg.expandTests(tm)
	if len(errs) > 0 {
		return nil, errs
	}

	// If we're not using depepndencies, we're done.
	if !config.UseDeps {
		return tg, nil
	}

	// If we are using dependencies, we need to do some more work:
	// 	1. Expand the dependencies and make sure everything is in order.
	//  2. Make sure there aren't any cycles in the dependency graph
	//  3. Pluck out all the sub trees for the requested tests

	g, errs := tm.dependencyGraph()
	if len(errs) > 0 {
		// problems expanding dependencies
		return nil, errs
	}

	// topsort returns an error if there's a cycle
	if _, err := g.TopSort(); err != nil {
		return nil, []error{err}
	}

	// Get the dependencies for everything already in the test group
	resChan := make(chan *expandRes)
	for id, _ := range tg.Tests {
		go getDepNodes(g, id, resChan)
	}

	// Finally, add the dependencies to the TestGroup's map
	for i, count := 0, len(tg.Tests); i < count; i++ {
		res := <-resChan
		for _, test := range res.Tests {
			if _, ok := tg.Tests[test.DependencyID]; !ok {
				test.IsDependency = true
				tg.Tests[test.DependencyID] = test
			}
		}
	}

	return tg, nil
}

func (t *TestGroup) TotalPoints() (points uint) {
	points = 0
	for _, test := range t.Tests {
		points += test.PointsAvailable
	}
	return
}

func (t *TestGroup) EarnedPoints() (points uint) {
	points = 0
	for _, test := range t.Tests {
		points += test.PointsEarned
	}
	return
}
