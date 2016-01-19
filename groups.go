package test161

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ops-class/test161/graph"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"
)

type CompletedTestHandler func(test *Test, res *TestResult)

// GroupConfig specifies how a group of tests should be created and run.
// A TestGroup will be created and run using
type GroupConfig struct {
	Name    string   `json:name`
	RootDir string   `json:rootdir`
	UseDeps bool     `json:"usedeps"`
	TestDir string   `json:"testdir"`
	Tags    []string `json:"tags"`
	Tests   []string `json:"-"`
}

// A group of tests to be run
type TestGroup struct {
	id        uint64
	Tests     []*Test
	Config    GroupConfig
	Callbacks []CompletedTestHandler
}

type jsonTestGroup struct {
	Id     uint64      `json:"id"`
	Config GroupConfig `json:"config"`
	Tests  []*Test     `json:"tests"`
}

var idLock = &sync.Mutex{}
var curID uint64 = 1

// Id retrieves the group id
func (t *TestGroup) Id() uint64 {
	return t.id
}

// Custom JSON marshaling to deal with our read-only id
func (tg *TestGroup) MarshalJSON() ([]byte, error) {
	return json.Marshal(jsonTestGroup{tg.Id(), tg.Config, tg.Tests})
}

// Increments the global counter and returns the previous value
func incrementId() (res uint64) {
	idLock.Lock()
	res = curID
	curID += 1
	if curID == 0 {
		curID = 1
	}
	idLock.Unlock()
	return
}

// EmptyGroup creates an empty TestGroup that can be used to add
// groups from strings.
func EmptyGroup() *TestGroup {
	tg := &TestGroup{}
	tg.id = incrementId()
	tg.Tests = make([]*Test, 0)
	return tg
}

func isTestFile(name string) bool {
	//TODO change this to whatever convention we want to use
	return strings.HasSuffix(name, ".yaml")
}

// Return a slice of Tests by reading all tests in the given
// directory and comparing the tags. If no tags are provided,
// all tests will be loaded.
//
// Other behavior:
//  - the function will recursively process directories
//  - symlinks are avoided
func loadTestsFromDir(dir string, tags []string) ([]*Test, error) {
	info, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	tests := make([]*Test, 0)

	for _, f := range info {
		// We're not even going to bother with symlinks
		if f.Mode()&os.ModeSymlink == os.ModeSymlink {
			continue
		}

		if f.IsDir() {
			if more, err := loadTestsFromDir(path.Join(dir, f.Name()), tags); err != nil {
				return nil, err
			} else if len(more) > 0 {
				tests = append(tests, more...)
			}
		} else if isTestFile(f.Name()) {
			test, err := TestFromFile(path.Join(dir, f.Name()))
			if err != nil {
				// If one of the test files has an error,
				// don't create the group.
				return nil, err
			}

			add := true

			// O(|tags| * |test.Tags|) because there really shouldn't be a ton of tags here
			if len(tags) > 0 {
				add = false
				for i := range test.Tags {
					for j := range tags {
						if strings.TrimSpace(test.Tags[i]) == strings.TrimSpace(tags[j]) {
							add = true
							break
						}
					}
					if add {
						break
					}
				}
			}
			if add {
				tests = append(tests, test)
			}
		}
	}
	return tests, nil
}

func GroupFromConfig(config GroupConfig) (*TestGroup, error) {
	tg := EmptyGroup()
	tg.Config = config

	// First, try loading from the test dir
	if strings.TrimSpace(config.TestDir) != "" {
		tests, err := loadTestsFromDir(config.TestDir, config.Tags)
		if err != nil {
			return nil, err
		}
		if len(tests) > 0 {
			tg.Tests = append(tg.Tests, tests...)
		}
	}

	// Next, add any additional tests
	for _, s := range config.Tests {
		test, err := TestFromString(s)
		if err != nil {
			return nil, err
		}
		tg.Tests = append(tg.Tests, test)
	}
	return tg, nil
}

func (t *TestGroup) VerifyDependencies() ([]string, error) {
	// create a map of test name
	m := make(map[string]*Test)
	for _, t := range t.Tests {
		if strings.TrimSpace(t.Name) == "" {
			return nil, errors.New("Unnamed test detected")
		}
		if _, ok := m[t.Name]; ok {
			return nil, errors.New(fmt.Sprintf("Duplicate test name detected:  %v", t.Name))
		}
		m[t.Name] = t
	}

	// verify each test's dependencies are in the map
	for _, test := range m {
		for _, dep := range test.Depends {
			if _, ok := m[dep]; !ok {
				return nil, errors.New(fmt.Sprintf("Cannot find dependency %v for test %v", dep, test.Name))
			}
		}
	}

	// verify it's a DAG
	nodes := make([]string, 0, len(m))
	for testName := range m {
		nodes = append(nodes, testName)
	}
	g := graph.New(nodes)

	// add edges
	for _, test := range m {
		for _, dep := range test.Depends {
			// edge from test -> dep
			if err := g.AddEdge(test.Name, dep); err != nil {
				return nil, err
			}
		}
	}

	// If TopSort returns an error, we have a cycle
	if sort, err := g.TopSort(); err != nil {
		return nil, err
	} else {
		return sort, nil
	}
}

func (t *TestGroup) CanRun() bool {
	if len(t.Tests) == 0 {
		return false
	}

	if t.Config.UseDeps {
		if _, err := t.VerifyDependencies(); err != nil {
			return false
		}
	}

	// other tests here...

	return true
}
