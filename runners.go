package test161

type TestRunner interface {
	Run()
}

type SimpleRunner struct {
	Group *TestGroup

	// Outgoing, non-blocking, buffered channel
	// Set to nil or cap 0 if you don't want results
	// for some reason
	CompletedChan chan *test161JobResult
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
