package test161

import (
	"errors"
	"fmt"
	"sync"
	"time"
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
	Running     uint  `json:"running"`
	HighRunning uint  `json:"high_running"`
	Queued      uint  `json:"queued"`
	HighQueued  uint  `json:"high_queued"`
	Finished    uint  `json:"finished"`
	MaxWait     int64 `json:"max_wait_ms"`
	AvgWait     int64 `json:"avg_wait_ms"`
	total       int64 // denominator for avg
}

// Combined submission and tests statistics since the service started
type Test161Stats struct {
	Status          string       `json:"status"`
	SubmissionStats ManagerStats `json:"submission_stats"`
	TestStats       ManagerStats `json:"test_stats"`
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
	start := time.Now()

	for m.Capacity > 0 && m.stats.Running >= m.Capacity {
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

		// Max and average waits
		curWait := int64(time.Now().Sub(start).Nanoseconds() / 1e6)
		if m.stats.MaxWait < curWait {
			m.stats.MaxWait = curWait
		}
		m.stats.AvgWait = (m.stats.total*m.stats.AvgWait + curWait) / (m.stats.total + 1)
		m.stats.total += 1

		m.statsCond.Broadcast()
	}

	m.stats.Running += 1
	if m.stats.Running > m.stats.HighRunning {
		m.stats.HighRunning = m.stats.Running
	}

	m.statsCond.L.Unlock()

	// Go!
	err := job.Test.Run(job.Env)

	// And... we're done.

	// Update stats
	m.statsCond.L.Lock()
	m.stats.Running -= 1
	m.stats.Finished += 1

	// Broadcast because different entities are blocking for different reasons
	m.statsCond.Broadcast()
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

func (m *manager) Stats() *ManagerStats {
	// Lock so we at least get a consistent view of the stats
	testManager.statsCond.L.Lock()
	defer testManager.statsCond.L.Unlock()

	// copy
	var res ManagerStats = testManager.stats
	return &res
}

// Return a copy of the current shared test manager stats
func GetManagerStats() *ManagerStats {
	return testManager.Stats()
}

////////  Submission Manager
//
// SubmissionManager handles running multiple submissions, which is useful for the server.
// The (Test)Manager already handles rate limiting tests, but we also need to be careful
// about rate limiting submissions.  In particular, we don't need to build all new
// if we have queued tests.  This just wastes cycles and I/O that the tests could use.
// Plus, the student will see the status go from building to running, but the boot test
// will just get queued.

const (
	SM_ACCEPTING = iota
	SM_NOT_ACCEPTING
)

type SubmissionManager struct {
	env     *TestEnvironment
	runlock *sync.Mutex // Block everything from running
	l       *sync.Mutex // Synchronize other state
	status  int
	stats   ManagerStats
}

func NewSubmissionManager(env *TestEnvironment) *SubmissionManager {
	mgr := &SubmissionManager{
		env:     env,
		runlock: &sync.Mutex{},
		l:       &sync.Mutex{},
		status:  SM_ACCEPTING,
	}
	return mgr
}

func (sm *SubmissionManager) CombinedStats() *Test161Stats {
	stats := &Test161Stats{
		SubmissionStats: *sm.Stats(),
		TestStats:       *sm.env.manager.Stats(),
	}
	if sm.Status() == SM_ACCEPTING {
		stats.Status = "accepting submissions"
	} else {
		stats.Status = "not accepting submissions"
	}
	return stats
}

func (sm *SubmissionManager) Stats() *ManagerStats {
	sm.l.Lock()
	defer sm.l.Unlock()
	copy := sm.stats
	return &copy
}

func (sm *SubmissionManager) Run(s *Submission) error {

	// The test manager we're associated with
	mgr := sm.env.manager

	sm.l.Lock()

	// Check to see if we've been paused or stopped. The server checks too, but there's delay.
	if sm.status != SM_ACCEPTING {
		sm.l.Unlock()
		s.Status = SUBMISSION_ABORTED
		err := errors.New("The submission server is not accepting new submissions at this time")
		s.Errors = append(s.Errors, fmt.Sprintf("%v", err))
		sm.env.notifyAndLogErr("Submissions Closed", s, MSG_PERSIST_COMPLETE, 0)
		return err
	}

	// Update Queued
	sm.stats.Queued += 1
	start := time.Now()
	if sm.stats.HighQueued < sm.stats.Queued {
		sm.stats.HighQueued = sm.stats.Queued
	}
	sm.l.Unlock()

	///////////
	// Queued here
	sm.runlock.Lock()

	// Still queued, but on deck. Wait on the manager's condition variable so we
	// get notifications when stats change.
	mgr.statsCond.L.Lock()
	for mgr.stats.Queued > 0 {
		mgr.statsCond.Wait()
	}
	mgr.statsCond.L.Unlock()
	// It's OK if we get a rush of builds here. Eventually we'll get a queue again, and
	// the test manager handles this.
	///////////

	// Update run stats
	sm.l.Lock()
	sm.stats.Queued -= 1
	sm.stats.Running += 1
	if sm.stats.HighRunning < sm.stats.Running {
		sm.stats.HighRunning = sm.stats.Running
	}

	// Max and average waits
	curWait := int64(time.Now().Sub(start).Nanoseconds() / 1e6)
	if sm.stats.MaxWait < curWait {
		sm.stats.MaxWait = curWait
	}
	sm.stats.AvgWait = (sm.stats.total*sm.stats.AvgWait + curWait) / (sm.stats.total + 1)
	sm.stats.total += 1

	sm.l.Unlock()

	// Run the submission
	sm.runlock.Unlock()
	err := s.Run()

	// Update stats
	sm.l.Lock()
	sm.stats.Running -= 1
	sm.stats.Finished += 1
	sm.l.Unlock()

	return err
}

func (sm *SubmissionManager) Pause() {
	sm.l.Lock()
	defer sm.l.Unlock()
	sm.status = SM_NOT_ACCEPTING
}

func (sm *SubmissionManager) Resume() {
	sm.l.Lock()
	defer sm.l.Unlock()
	sm.status = SM_ACCEPTING
}

func (sm *SubmissionManager) Status() int {
	sm.l.Lock()
	defer sm.l.Unlock()
	return sm.status
}
