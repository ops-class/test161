package test161

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func runnerFromConfig(t *testing.T, config *GroupConfig, expected []string) TestRunner {

	// Create a test the group
	r, errs := TestRunnerFromConfig(config)
	assert.Equal(t, 0, len(errs))
	if len(errs) > 0 {
		t.Log(errs)
		return nil
	}

	tg := r.Group()
	assert.NotNil(t, r.Group())
	assert.Equal(t, len(expected), len(tg.Tests))

	switch r.(type) {
	case *SimpleRunner:
		assert.False(t, config.UseDeps)
	case *DependencyRunner:
		assert.True(t, config.UseDeps)
	default:
		t.Errorf("Unexpected type for runner r")
	}

	// Make sure it has what we're expecting
	for _, id := range expected {
		test, ok := tg.Tests[id]
		assert.True(t, ok)
		if ok {
			assert.Equal(t, id, test.DependencyID)
		}
	}

	t.Log(tg)

	return r
}

func TestRunnerCapacity(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	// Copy the default environment so we can have our own manager
	env := defaultEnv.CopyEnvironment()
	env.manager = newManager()
	env.RootDir = "./fixtures/root"

	expected := []string{
		"boot.t", "threads/tt1.t", "threads/tt2.t", "threads/tt3.t",
		"sync/sy1.t", "sync/sy2.t", "sync/semu1.t",
	}

	config := &GroupConfig{
		Name:    "Test",
		UseDeps: false,
		Tests:   expected,
		Env:     env,
	}

	caps := []uint{0, 1, 3, 5}

	for i := 0; i < 4; i++ {
		env.manager.Capacity = caps[i]
		r := runnerFromConfig(t, config, expected)

		env.manager.start()

		done := r.Run()
		count := 0

		for res := range done {
			assert.Nil(res.Err)
			assert.Equal(TEST_RESULT_CORRECT, res.Test.Result)
			if res.Test.Result != TEST_RESULT_CORRECT {
				t.Log(res.Err)
				t.Log(res.Test.OutputJSON())
				t.Log(res.Test.OutputString())
			}
			t.Log(fmt.Sprintf("test: %v  status: %v", res.Test.DependencyID, res.Test.Result))
			count += 1
		}

		assert.Equal(len(expected), count)
		assert.Equal(uint(len(expected)), env.manager.stats.Finished)

		if env.manager.Capacity > 0 {
			assert.True(env.manager.stats.HighCount <= env.manager.Capacity)
		}

		t.Log(fmt.Sprintf("High count: %v High queue: %v Finished: %v",
			env.manager.stats.HighCount, env.manager.stats.HighQueued, env.manager.stats.Finished))

		env.manager.stop()
	}
}

func TestRunnerSimple(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	env := defaultEnv.CopyEnvironment()
	env.manager = newManager()
	env.RootDir = "./fixtures/root"

	expected := []string{
		"threads/tt1.t", "sync/sy1.t",
	}

	// Test config with dependencies
	config := &GroupConfig{
		Name:    "Test",
		UseDeps: false,
		Tests:   expected,
		Env:     env,
	}

	r := runnerFromConfig(t, config, expected)

	env.manager.start()

	done := r.Run()
	count := 0

	for res := range done {
		assert.Nil(res.Err)
		assert.Equal(TEST_RESULT_CORRECT, res.Test.Result)
		if TEST_RESULT_CORRECT != res.Test.Result {
			t.Log(res.Err)
			t.Log(res.Test.OutputJSON())
			t.Log(res.Test.OutputString())
		}
		count += 1
	}

	assert.Equal(len(expected), count)
	assert.Equal(uint(len(expected)), env.manager.stats.Finished)

	// Shut it down
	env.manager.stop()
}

func TestRunnerDependency(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	expected := []string{
		"boot.t", "threads/tt1.t", "threads/tt2.t", "threads/tt3.t",
		"sync/sy2.t", "sync/sy3.t", "sync/sy4.t",
	}

	env := defaultEnv.CopyEnvironment()
	env.manager = newManager()
	env.RootDir = "./fixtures/root"

	config := &GroupConfig{
		Name:    "Test",
		UseDeps: true,
		Tests:   []string{"sync/sy4.t"},
		Env:     env,
	}

	r := runnerFromConfig(t, config, expected)
	env.manager.Capacity = 0
	env.manager.start()
	done := r.Run()

	results := make([]string, 0)
	count := 0

	for res := range done {
		assert.Nil(res.Err)
		assert.Equal(TEST_RESULT_CORRECT, res.Test.Result)
		if res.Test.Result != TEST_RESULT_CORRECT {
			t.Log(res.Err)
			t.Log(res.Test.OutputJSON())
			t.Log(res.Test.OutputString())
		}
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

	env.manager.stop()
}

func TestRunnerAbort(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	expected := []string{
		"boot.t", "panics/panic.t", "panics/deppanic.t",
	}

	env := defaultEnv.CopyEnvironment()
	env.manager = newManager()
	env.RootDir = "./fixtures/root"

	config := &GroupConfig{
		Name:    "Test",
		UseDeps: true,
		Tests:   []string{"panics/deppanic.t"},
		Env:     env,
	}

	r := runnerFromConfig(t, config, expected)
	env.manager.Capacity = 0
	env.manager.start()
	done := r.Run()

	count := 0
	for res := range done {
		assert.Nil(res.Err)
		assert.Equal(expected[count], res.Test.DependencyID)

		var expected TestResult

		switch count {
		case 0: // boot
			expected = TEST_RESULT_CORRECT
		case 1: // panic
			expected = TEST_RESULT_INCORRECT
		case 2: // deppanic
			expected = TEST_RESULT_SKIP
		}

		count += 1

		assert.Equal(expected, res.Test.Result)
		if expected != res.Test.Result {
			t.Log(res.Err)
			t.Log(res.Test.OutputJSON())
			t.Log(res.Test.OutputString())
		}
	}

	assert.Equal(len(expected), count)

	env.manager.stop()
}

func TestRunnersParallel(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	env := defaultEnv.CopyEnvironment()
	env.manager = newManager()
	env.RootDir = "./fixtures/root"

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

	runners := make([]TestRunner, 0, len(tests))

	for _, group := range tests {
		config := &GroupConfig{
			Name:    "Test",
			UseDeps: false,
			Tests:   group,
			Env:     env,
		}
		r := runnerFromConfig(t, config, group)
		runners = append(runners, r)
	}

	syncChan := make(chan int)

	env.manager.Capacity = 10
	env.manager.start()

	for index, runner := range runners {
		go func(r TestRunner, i int) {
			done := r.Run()
			count := 0

			for res := range done {
				assert.Nil(res.Err)
				assert.Equal(TEST_RESULT_CORRECT, res.Test.Result)
				if res.Test.Result != TEST_RESULT_CORRECT {
					t.Log(res.Err)
					t.Log(res.Test.OutputJSON())
					t.Log(res.Test.OutputString())
				}

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

	env.manager.stop()

	if env.manager.Capacity > 0 {
		assert.True(env.manager.stats.HighCount <= env.manager.Capacity)
	}

	t.Log(fmt.Sprintf("High count: %v High queue: %v Finished: %v",
		env.manager.stats.HighCount, env.manager.stats.HighQueued, env.manager.stats.Finished))

}
