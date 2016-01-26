package test161

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func runnerFromConfig(t *testing.T, config *GroupConfig, expected []string) TestRunner {

	// Create a test the group
	tg, errs := GroupFromConfig(config)
	assert.Equal(t, 0, len(errs))
	if len(errs) > 0 {
		t.Log(errs)
	}

	assert.NotNil(t, tg)

	assert.Equal(t, len(expected), len(tg.Tests))

	// Make sure it has what we're expecting
	for _, id := range expected {
		test, ok := tg.Tests[id]
		assert.True(t, ok)
		if ok {
			assert.Equal(t, id, test.DependencyID)
		}
	}

	done := make(chan *test161JobResult, len(tg.Tests))
	var r TestRunner

	if config.UseDeps {
		r = &DependencyRunner{tg, done}
	} else {
		r = &DependencyRunner{tg, done}
	}

	return r
}

func TestRunnerCapacity(t *testing.T) {
	// Not parallel since we're changing the capacity!
	//t.Parallel()
	assert := assert.New(t)

	expected := []string{
		"boot.t", "threads/tt1.t", "threads/tt2.t", "threads/tt3.t",
		"sync/sy1.t", "sync/sy2.t", "sync/semu1.t",
	}

	config := &GroupConfig{
		Name:    "Test",
		RootDir: "./fixtures/root",
		UseDeps: false,
		TestDir: TEST_DIR,
		Tests:   expected,
	}

	for i := 0; i < 4; i++ {
		switch i {
		case 0:
			taskManager.Capacity = 0
		case 1:
			taskManager.Capacity = 1
		case 2:
			taskManager.Capacity = 3
		case 3:
			taskManager.Capacity = 5
		}

		r := runnerFromConfig(t, config, expected)

		taskManager.stop() //clear stats
		taskManager.start()

		r.Run()

		count := 0
		done := r.GetCompletedChan()
		for res := range done {
			assert.Nil(res.Err)
			assert.Equal(T_RES_OK, res.Test.Result)
			t.Log(fmt.Sprintf("test: %v  status: %v", res.Test.DependencyID, res.Test.Result))
			count += 1
		}

		assert.Equal(len(expected), count)
		assert.Equal(uint(len(expected)), taskManager.stats.Finished)

		if taskManager.Capacity > 0 {
			assert.True(taskManager.stats.HighCount <= taskManager.Capacity)
		}
		t.Log(fmt.Sprintf("High count: %v High queue: %v Finished: %v",
			taskManager.stats.HighCount, taskManager.stats.HighQueued, taskManager.stats.Finished))

		taskManager.stop()
	}

	taskManager.Capacity = 0
}

func TestRunnerSimple(t *testing.T) {
	// Also not parallel because we need to start/stop the manager
	assert := assert.New(t)

	expected := []string{
		"threads/tt1.t", "sync/sy1.t",
	}

	// Test config with dependencies
	config := &GroupConfig{
		Name:    "Test",
		RootDir: "./fixtures/root",
		UseDeps: false,
		TestDir: TEST_DIR,
		Tests:   expected,
	}

	r := runnerFromConfig(t, config, expected)

	taskManager.start()

	r.Run()

	count := 0
	done := r.GetCompletedChan()

	for res := range done {
		assert.Nil(res.Err)
		assert.Equal(T_RES_OK, res.Test.Result)
		t.Log(res.Test.OutputString())
		t.Log(res.Test.OutputJSON())
		count += 1
	}

	assert.Equal(len(expected), count)
	assert.Equal(uint(len(expected)), taskManager.stats.Finished)

	// Shut it down
	taskManager.stop()
}

func TestRunnerDependency(t *testing.T) {
	//No parallel here either

	assert := assert.New(t)

	expected := []string{
		"boot.t", "threads/tt1.t", "threads/tt2.t", "threads/tt3.t",
		"sync/sy2.t", "sync/sy3.t", "sync/sy4.t",
	}

	config := &GroupConfig{
		Name:    "Test",
		RootDir: "./fixtures/root",
		UseDeps: true,
		TestDir: TEST_DIR,
		Tests:   []string{"sync/sy4.t"},
	}

	r := runnerFromConfig(t, config, expected)
	taskManager.Capacity = 0
	taskManager.start()
	r.Run()

	results := make([]string, 0)
	count := 0
	done := r.GetCompletedChan()

	for res := range done {
		assert.Nil(res.Err)
		assert.Equal(T_RES_OK, res.Test.Result)
		t.Log(res.Test.OutputString())
		t.Log(res.Test.OutputJSON())
		count += 1
		results = append(results, res.Test.DependencyID)
	}

	assert.Equal(len(expected), count)

	// Boot has to be first, and since sy4 depends on sy3 depends on threads,
	// sy4 needs to be last and sy3 needs to be second to last.  Finally,
	// sy3 depends on locks (sy2), so that is third from the end.
	assert.Equal(expected[0], results[0])
	assert.Equal(expected[len(expected)-3], results[len(expected)-3])
	assert.Equal(expected[len(expected)-2], results[len(expected)-2])
	assert.Equal(expected[len(expected)-1], results[len(expected)-1])

	taskManager.stop()
}

func TestRunnerAbort(t *testing.T) {
	//No parallel here either

	assert := assert.New(t)

	expected := []string{
		"boot.t", "panics/panic.t", "panics/deppanic.t",
	}

	config := &GroupConfig{
		Name:    "Test",
		RootDir: "./fixtures/root",
		UseDeps: true,
		TestDir: TEST_DIR,
		Tests:   []string{"panics/deppanic.t"},
	}

	r := runnerFromConfig(t, config, expected)
	taskManager.Capacity = 0
	taskManager.start()
	r.Run()
	done := r.GetCompletedChan()

	count := 0
	for res := range done {
		assert.Nil(res.Err)
		assert.Equal(expected[count], res.Test.DependencyID)

		switch count {
		case 0: // boot
			assert.Equal(T_RES_OK, res.Test.Result)
		case 1: // panic
			assert.Equal(T_RES_FAIL, res.Test.Result)
		case 2: // deppanic
			assert.Equal(T_RES_SKIP, res.Test.Result)
		}
		count += 1

		t.Log(res.Test.OutputString())
		t.Log(res.Test.OutputJSON())
	}

	assert.Equal(len(expected), count)

	taskManager.stop()
}

func TestRunnersParallel(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	tests := [][]string{
		[]string{
			"boot.t", "sync/sy2.t", "sync/sy3.t", "threads/tt1.t",
		},
		[]string{
			"boot.t", "threads/tt1.t", "threads/tt3.t", "sync/sy1.t", "sync/sy2.t",
		},
		[]string{
			"boot.t", "threads/tt1.t", "threads/tt3.t", "sync/sy1.t", "sync/sy2.t",
		},
		[]string{
			"boot.t", "threads/tt1.t", "threads/tt3.t", "sync/sy1.t", "sync/sy2.t", "sync/sy3.t", "sync/sy4.t",
		},
		[]string{
			"boot.t", "threads/tt1.t", "threads/tt3.t", "sync/sy1.t", "sync/sy2.t", "sync/sy4.t", "sync/semu1.t",
		},
	}

	taskManager.Capacity = 5
	taskManager.stop()
	taskManager.start()

	runners := make([]TestRunner, 0, len(tests))

	for _, group := range tests {
		config := &GroupConfig{
			Name:    "Test",
			RootDir: "./fixtures/root",
			UseDeps: false,
			TestDir: TEST_DIR,
			Tests:   group,
		}
		r := runnerFromConfig(t, config, group)
		runners = append(runners, r)
	}

	syncChan := make(chan int)

	taskManager.Capacity = 10
	taskManager.stop()
	taskManager.start()

	for index, runner := range runners {
		go func(r TestRunner, i int) {
			r.Run()

			count := 0
			done := r.GetCompletedChan()

			for res := range done {
				assert.Nil(res.Err)
				assert.Equal(T_RES_OK, res.Test.Result)
				t.Log(res.Test.OutputString())
				t.Log(res.Test.OutputJSON())

				count += 1
			}

			// Done with this test group
			assert.Equal(len(tests[i]), count)
			syncChan <- 1
		}(runner, index)
	}

	// Let all the workers finish
	for i, count := 0, len(runners); i < count; i++ {
		<-syncChan
	}

	taskManager.stop()

	if taskManager.Capacity > 0 {
		assert.True(taskManager.stats.HighCount <= taskManager.Capacity)
	}

	t.Log(fmt.Sprintf("High count: %v High queue: %v Finished: %v",
		taskManager.stats.HighCount, taskManager.stats.HighQueued, taskManager.stats.Finished))

}
