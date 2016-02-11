package test161

import (
	"sync"
)

// This file defines test161's test manager.  The manager is responsible for
// keeping track of the number of running tests and limiting that number to
// a configurable capacity.
//
// There is a global test manager (testManager) that listens for new job
// requests on its SubmitChan.  In the current implementation, this can
// only be accessed from within the package by one of the TestRunners.

// A test161Job consists of the test to run, the directory to find the
// binaries, and a channel to communicate the results on.
type test161Job struct {
	Test     *Test
	Env      *TestEnvironment
	DoneChan chan *Test161JobResult
}

// A Test161JobResult consists of the completed test and any error that
// occurred while running the test.
type Test161JobResult struct {
	Test *Test
	Err  error
}

type manager struct {
	SubmitChan chan *test161Job
	Capacity   uint

	// protected by L
	statsCond *sync.Cond
	isRunning bool

	stats ManagerStats
}

type ManagerStats struct {
	// protected by manager.L
	NumRunning uint
	HighCount  uint
	Queued     uint
	HighQueued uint
	Finished   uint
}

const DEFAULT_MGR_CAPACITY uint = 0

func newManager() *manager {
	m := &manager{
		SubmitChan: nil,
		Capacity:   DEFAULT_MGR_CAPACITY,
		statsCond:  sync.NewCond(&sync.Mutex{}),
		isRunning:  false,
	}
	return m
}

// The global test manager.  NewEnvironment has a reference to
// this manager, which should be used by clients of this library.
// Unit tests use their own manager/environment for runner/assertion
// isolation.
var testManager *manager = newManager()

// Clear state and start listening for job requests
func (m *manager) start() {
	m.statsCond.L.Lock()
	defer m.statsCond.L.Unlock()

	if m.isRunning {
		return
	}

	m.stats = ManagerStats{}
	m.SubmitChan = make(chan *test161Job)
	m.isRunning = true

	// Listener goroutine
	go func() {
		// We simply spawn a worker that blocks until it can run.
		for job := range m.SubmitChan {
			go m.runOrQueueJob(job)
		}
	}()
}

// Queue the job if we're at capacity, and run it once we're under.
func (m *manager) runOrQueueJob(job *test161Job) {

	m.statsCond.L.Lock()
	queued := false

	for m.Capacity > 0 && m.stats.NumRunning >= m.Capacity {
		if !queued {
			queued = true

			// Update queued stats
			m.stats.Queued += 1
			if m.stats.Queued > m.stats.HighQueued {
				m.stats.HighQueued = m.stats.Queued
			}
		}

		// Wait for a finished test to signal us
		m.statsCond.Wait()
	}

	// We've got the green light... (and the stats lock)
	if queued {
		m.stats.Queued -= 1
		queued = false
	}

	m.stats.NumRunning += 1
	if m.stats.NumRunning > m.stats.HighCount {
		m.stats.HighCount = m.stats.NumRunning
	}

	m.statsCond.L.Unlock()

	// Go!
	err := job.Test.Run(job.Env)

	// And... we're done.

	// Update stats
	m.statsCond.L.Lock()
	m.stats.NumRunning -= 1
	m.stats.Finished += 1
	m.statsCond.Signal()
	m.statsCond.L.Unlock()

	// Pass the completed test back to the caller
	// (Blocking call, we need to make sure the caller gets the result.)
	job.DoneChan <- &Test161JobResult{job.Test, err}
}

// Shut it down
func (m *manager) stop() {
	m.statsCond.L.Lock()
	defer m.statsCond.L.Unlock()

	if !m.isRunning {
		return
	}

	m.isRunning = false
	close(m.SubmitChan)
}

// Exported shared test manger functions

// Start the shared test manager
func StartManager() {
	testManager.start()
}

// Stop the shared test manager
func StopManager() {
	testManager.stop()
}

func SetManagerCapacity(capacity uint) {
	testManager.Capacity = capacity
}

func ManagerCapacity() uint {
	return testManager.Capacity
}

// Return a copy of the current shared test manager stats
func GetManagerStats() *ManagerStats {
	// Lock so we at least get a consistent view of the stats
	testManager.statsCond.L.Lock()
	defer testManager.statsCond.L.Unlock()

	// copy
	var res ManagerStats = testManager.stats
	return &res
}
