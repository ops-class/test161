package test161

// A TestRunner is responsible for running a TestGroup and sending the
// results back on a read-only channel. test161 runners close the results
// channel when finished so clients can range over it. test161 runners also
// return as soon as they are able to and let tests run asynchronously.
type TestRunner interface {
	Group() *TestGroup
	Run() <-chan *Test161JobResult
}

// Create a TestRunner from a GroupConfig.  config.UseDeps determines the
// type of runner created.
func TestRunnerFromConfig(config *GroupConfig) (TestRunner, []error) {
	if tg, errs := GroupFromConfig(config); len(errs) > 0 {
		return nil, errs
	} else if config.UseDeps {
		return NewDependencyRunner(tg), nil
	} else {
		return NewSimpleRunner(tg), nil
	}
}

// Factory function to create a new SimpleRunner.
func NewSimpleRunner(group *TestGroup) TestRunner {
	return &SimpleRunner{group}
}

// Factory function to create a new DependencyRunner.
func NewDependencyRunner(group *TestGroup) TestRunner {
	return &DependencyRunner{group}
}

// A simple runner that tries to run everything as fast as it's allowed to,
// i.e. it doesn't care about dependencies.
type SimpleRunner struct {
	group *TestGroup
}

func (r *SimpleRunner) Group() *TestGroup {
	return r.group
}

func (r *SimpleRunner) Run() <-chan *Test161JobResult {

	// We create 2 channels, one to receive the results from the test
	// manager and one to transmit the results to the caller.  We
	// don't just pass the caller channel to the test manager because
	// we promise to close the channel when we're done so clients can
	// range over the channel if they'd like

	// For retrieving results from the test manager
	resChan := make(chan *Test161JobResult)

	// Buffered channel for our client.
	callbackChan := make(chan *Test161JobResult, len(r.group.Tests))

	env := r.group.Config.Env

	// Spawn every job at once (no dependency tracking)
	for _, test := range r.group.Tests {
		job := &test161Job{test, env, resChan}
		env.manager.SubmitChan <- job
	}

	go func() {
		for i, count := 0, len(r.group.Tests); i < count; i++ {
			// Always block recieving the test result
			res := <-resChan

			// But, never block sending it back
			select {
			case callbackChan <- res:
			default:
			}
		}
		close(callbackChan)
	}()

	return callbackChan
}

// This runner has mad respect for dependencies.
type DependencyRunner struct {
	group *TestGroup
}

func (r *DependencyRunner) Group() *TestGroup {
	return r.group
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
		res := <-depChan
		if _, ok := deps[res.DependencyID]; ok {
			if res.Result == TEST_RESULT_CORRECT {
				delete(deps, res.DependencyID)
			} else {
				test.Result = TEST_RESULT_SKIP
				abortChan <- test
			}
		}
	}

	// We're clear
	readyChan <- test
}

func (r *DependencyRunner) Run() <-chan *Test161JobResult {

	// Everything that's still waiting.
	// We make it big enough that it can hold all the results
	waiting := make(map[string]chan *Test, len(r.group.Tests))

	// The channel that our goroutines message us back on
	// to let us know they're ready
	readyChan := make(chan *Test)

	// The channel we use to get results back from the manager
	resChan := make(chan *Test161JobResult)

	// The channel that our goroutines message us back on
	// when they can't run because a dependency failed.
	abortChan := make(chan *Test)

	// A function to broadcast a single test result
	bcast := func(test *Test) {
		for _, ch := range waiting {
			// Non-blocking because it
			select {
			case ch <- test:
			default:
			}
		}
	}

	// Buffered channel for our client.
	callbackChan := make(chan *Test161JobResult, len(r.group.Tests))

	// A function to send the result back to the caller
	callback := func(res *Test161JobResult) {
		// Don't block, not that we should since we're buffered
		select {
		case callbackChan <- res:
		default:
		}
	}

	// Spawn all the tests and put them in a waiting pattern
	for id, test := range r.group.Tests {
		waiting[id] = make(chan *Test)
		go waitForDeps(test, waiting[id], readyChan, abortChan)
	}

	// Main goroutine responsible for directing traffic.
	//  - Results from the manager and abort channels are broadcast to the
	//    remaining blocked tests and caller
	//  - Tests coming in on the ready channel get sent to the manager
	go func() {
		//We're done as soon as we recieve the final result from the manager.
		results := 0
		env := r.group.Config.Env

		for results < len(r.group.Tests) {
			select {
			case res := <-resChan:
				bcast(res.Test)
				callback(res)
				results += 1

			case test := <-abortChan:
				// Abort!
				delete(waiting, test.DependencyID)
				bcast(test)
				callback(&Test161JobResult{test, nil})
				results += 1

			case test := <-readyChan:
				// We have a test that can run.
				delete(waiting, test.DependencyID)
				job := &test161Job{test, env, resChan}
				env.manager.SubmitChan <- job
			}
		}
		close(callbackChan)
	}()

	return callbackChan
}
