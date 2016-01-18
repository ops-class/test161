package test161

import (
	"errors"
	"fmt"
	"github.com/ops-class/test161/graph"
	"sync"
)

type TestGroup struct {
	// All the tests that should be run as part of this group
	Tests []*Test

	// The results channel to send completed test results.
	// This is not buffered and
	ResultsChan chan string

	// Private group id
	id uint64
}

var idLock = &sync.Mutex{}
var curID uint64 = 1

// Id retrieves the group id
func (t *TestGroup) Id() uint64 {
	return t.id
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
	tg.ResultsChan = make(chan string)
	return tg
}

// GroupFromDirectory creates a TestGroup from all test files
// in the given directory
func GroupFromDirectory(dir string) (*TestGroup, error) {
	return nil, errors.New("Not Implemented")
}

func (t *TestGroup) VerifyDependencies() ([]string, error) {
	// create a map of test name
	m := make(map[string]*Test)
	for _, t := range t.Tests {
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
			g.AddEdge(test.Name, dep)
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

	if _, err := t.VerifyDependencies(); err != nil {
		return false
	}

	// other tests here...

	return true
}
