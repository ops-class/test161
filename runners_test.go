package test161

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRunnerSimple(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	// Test config with dependencies
	config := &GroupConfig{
		Name:    "Test",
		RootDir: "./fixtures",
		UseDeps: true,
		TestDir: TEST_DIR,
		Tests:   []string{"sync/sy1.t"},
	}

	expected := []string{
		"boot.t", "threads/tt1.t", "threads/tt2.t",
		"threads/tt3.t", "sync/sy1.t",
	}

	for i := 0; i < 2; i++ {
		tg, errs := GroupFromConfig(config)
		assert.Equal(0, len(errs))
		assert.NotNil(tg)

		assert.Equal(len(expected), len(tg.Tests))
		for _, id := range expected {
			test, ok := tg.Tests[id]
			assert.True(ok)
			if ok {
				assert.Equal(id, test.DependencyID)
			}
		}

		switch i {
		case 0:
			taskManager.Capacity = 10
		case 1:
			taskManager.Capacity = 1
		}

		taskManager.start()

		done := make(chan *test161JobResult, len(tg.Tests))
		r := &SimpleRunner{tg, done}
		r.Run()

		count := 0
		for res := range done {
			assert.Nil(res.Err)
			assert.Equal(T_RES_OK, res.Test.Result)
			fmt.Println("test", res.Test.DependencyID, "complete")
			t.Log(fmt.Sprintf("test: %v  status: %v", res.Test.DependencyID, res.Test.Result))
			count += 1
		}

		assert.True(taskManager.stats.HighCount <= taskManager.Capacity)

		t.Log(fmt.Sprintf("High count: %v", taskManager.stats.HighCount))
		t.Log(fmt.Sprintf("High queue: %v", taskManager.stats.HighQueued))

		// Shut it down
		taskManager.stop()
	}
}
