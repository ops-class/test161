package test161

import (
	"sync"
)

// None of this is exported outside of this package

type test161Job struct {
	Test     *Test
	RootDir  string
	DoneChan chan *test161JobResult
}

type test161JobResult struct {
	Test *Test
	Err  error
}

type manager struct {
	SubmitChan chan *test161Job
	Capacity   uint

	// protected by L
	statsCond *sync.Cond
	isRunning bool

	stats managerStats
}

type managerStats struct {
	// protected by l
	NumRunning uint
	HighCount  uint
	Queued     uint
	HighQueued uint
	Finished   uint
}

const DEFAULT_MGR_CAPACITY uint = 0

// There's only one manager, and this is it
var taskManager = &manager{
	SubmitChan: nil,
	Capacity:   DEFAULT_MGR_CAPACITY,
	statsCond:  sync.NewCond(&sync.Mutex{}),
	isRunning:  false,
}

func (m *manager) start() {
	m.statsCond.L.Lock()
	defer m.statsCond.L.Unlock()

	if m.isRunning {
		return
	}

	// reset stats
	m.stats = managerStats{}

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

func (m *manager) runOrQueueJob(job *test161Job) {

	// Wait until we're under capacity, and update queued stats
	m.statsCond.L.Lock()
	queued := false

	for m.Capacity > 0 && m.stats.NumRunning >= m.Capacity {
		if !queued {
			queued = true
			m.stats.Queued += 1
			if m.stats.Queued > m.stats.HighQueued {
				m.stats.HighQueued = m.stats.Queued
			}
		}
		m.statsCond.Wait()
	}

	// We've got the green light...
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
	err := job.Test.Run(job.RootDir)

	// And... we're done.

	// Update stats
	m.statsCond.L.Lock()
	m.stats.NumRunning -= 1
	m.stats.Finished += 1
	m.statsCond.Signal()
	m.statsCond.L.Unlock()

	// Pass the completed test back to the caller
	// (Blocking call, we need to make sure the caller gets the result.)
	job.DoneChan <- &test161JobResult{job.Test, err}
}

func (m *manager) stop() {
	m.statsCond.L.Lock()
	defer m.statsCond.L.Unlock()

	if !m.isRunning {
		return
	}

	m.isRunning = false
	close(m.SubmitChan)
}
