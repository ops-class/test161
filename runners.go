package test161

import (
	"log"
)

type TestRunner interface {
	Run()
	GetCompletedChan() chan *test161JobResult
}

// A simple runner that tries to run everything as fast as
// it's allowed to, i.e. doesn't care about dependencies.
type SimpleRunner struct {
	Group *TestGroup

	// Outgoing, non-blocking, buffered channel
	// Set to nil or cap 0 if you don't want results
	// for some reason
	CompletedChan chan *test161JobResult
}

func (r *SimpleRunner) GetCompletedChan() chan *test161JobResult {
	return r.CompletedChan
}

// Run a TestGroup Asynchronously
func (r *SimpleRunner) Run() {
	resChan := make(chan *test161JobResult)

	// Spawn every job at once (no dependency tracking)
	for _, test := range r.Group.Tests {
		job := &test161Job{test, r.Group.Config.RootDir, resChan}
		taskManager.SubmitChan <- job
	}

	go func() {
		for i, count := 0, len(r.Group.Tests); i < count; i++ {
			// Always block recieving the test result
			res := <-resChan

			// But, never block sending it back
			select {
			case r.CompletedChan <- res:
			default:
			}
		}
		close(r.CompletedChan)
	}()
}

// This runner has mad respect for dependencies
type DependencyRunner struct {
	Group         *TestGroup
	CompletedChan chan *test161JobResult
}

func (r *DependencyRunner) GetCompletedChan() chan *test161JobResult {
	return r.CompletedChan
}

// Holding pattern.  An individual test waits here until all of its
// dependencies have been met or failed, in which case it runs or aborts.
func waitForDeps(test *Test, depChan, readyChan, abortChan chan *Test) {
	// Copy deps
	deps := make(map[string]bool)
	for id := range test.ExpandedDeps {
		deps[id] = true
	}

	for len(deps) > 0 {
		log.Println(test.DependencyID, "waiting on dependencies")
		res := <-depChan
		if _, ok := deps[res.DependencyID]; ok {
			if res.Result == T_RES_OK {
				delete(deps, res.DependencyID)
			} else {
				test.Result = T_RES_SKIP
				abortChan <- test
			}
		}
	}

	log.Println(test.DependencyID, "dependencies met!")

	// We're clear
	readyChan <- test
}

func (r *DependencyRunner) Run() {

	// Everything that's still waiting
	// We make it big enough that it can hold all the results
	waiting := make(map[string]chan *Test, len(r.Group.Tests))

	// The channel that our goroutines message us back on
	// to let us know they're ready
	readyChan := make(chan *Test)

	// The channel we use to get results back from the manager
	resChan := make(chan *test161JobResult)

	// The channel that our goroutines message us back on
	// when they can't run because a dependency failed.
	abortChan := make(chan *Test)

	// Spawn all the tests and put them in a waiting pattern
	for id, test := range r.Group.Tests {
		waiting[id] = make(chan *Test)
		go waitForDeps(test, waiting[id], readyChan, abortChan)
	}

	// Broadcast the result
	bcast := func(test *Test) {
		for _, ch := range waiting {
			// Non-blocking because it
			select {
			case ch <- test:
			default:
			}
		}
	}

	callback := func(res *test161JobResult) {
		// But, never block sending it back
		select {
		case r.CompletedChan <- res:
		default:
		}
	}

	go func() {
		// We wait for a message from either the results channel (manager),
		// or the abort/ channels.  We're done as soon as we recieve the
		// final result from the manager.
		results := 0
		for results < len(r.Group.Tests) {
			select {
			case res := <-resChan:
				bcast(res.Test)
				callback(res)
				results += 1

			case test := <-abortChan:
				// Abort!
				delete(waiting, test.DependencyID)
				bcast(test)
				callback(&test161JobResult{test, nil})
				results += 1

			case test := <-readyChan:
				// We have a test that can run.
				delete(waiting, test.DependencyID)
				job := &test161Job{test, r.Group.Config.RootDir, resChan}
				taskManager.SubmitChan <- job
			}
		}
		close(r.CompletedChan)
	}()
}
