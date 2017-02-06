package test161

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTargetLoad(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	text := `---
name: asst1
points: 90
type: asst
tests:
  - id: sync/sem1.t
    points: 20
  - id: sync/lt1.t
    points: 30
  - id: sync/multi.t
    points: 40
    scoring: partial
    commands:
      - id: sem1
        points: 25
      - id: lt1
        points: 15
`
	target, err := TargetFromString(text)
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	assert.Equal(TARGET_ASST, target.Type)
	assert.Equal(3, len(target.Tests))
	if len(target.Tests) == 3 {
		assert.Equal("sync/sem1.t", target.Tests[0].Id)
		assert.Equal(TEST_SCORING_ENTIRE, target.Tests[0].Scoring)
		assert.Equal(uint(20), target.Tests[0].Points)

		assert.Equal("sync/lt1.t", target.Tests[1].Id)
		assert.Equal(TEST_SCORING_ENTIRE, target.Tests[1].Scoring)
		assert.Equal(uint(30), target.Tests[1].Points)

		assert.Equal("sync/multi.t", target.Tests[2].Id)
		assert.Equal(TEST_SCORING_PARTIAL, target.Tests[2].Scoring)
		assert.Equal(uint(40), target.Tests[2].Points)
	}

	tg, errs := target.Instance(defaultEnv)
	assert.Equal(0, len(errs))
	assert.NotNil(tg)
	t.Log(errs)
	t.Log(tg.OutputJSON())

}

type expectedCmdResults struct {
	points uint
	status string
}

type expectedTestResults struct {
	points    uint
	result    TestResult
	cmdPoints map[string]*expectedCmdResults
}

func runTargetTest(t *testing.T, testPoints map[string]*expectedTestResults, targetId string) *TestGroup {
	assert := assert.New(t)
	t.Log(targetId)

	env := defaultEnv.CopyEnvironment()
	env.manager = newManager()
	env.RootDir = "./fixtures/root"
	env.manager.Capacity = 10

	target, ok := defaultEnv.Targets[targetId]
	assert.True(ok)

	if !ok {
		t.FailNow()
	}

	tg, errs := target.Instance(env)
	assert.NotNil(tg)
	if tg == nil {
		t.Log(errs)
		t.FailNow()
	}

	assert.Equal(target.Points, tg.TotalPoints())

	totalExpected := uint(0)
	for _, v := range testPoints {
		totalExpected += v.points
	}

	// Run it and make sure we get the correct results
	env.manager.start()

	r := NewDependencyRunner(tg)
	done := r.Run()

	for res := range done {
		id := res.Test.DependencyID
		t.Log(id + " completed")
		if res.Err != nil {
			t.Log(res.Err)
		}
		exp, ok := testPoints[id]
		if !ok {
			assert.Equal(uint(0), res.Test.PointsEarned)
			assert.Equal(uint(0), res.Test.PointsAvailable)
			for _, c := range res.Test.Commands {
				assert.Equal(uint(0), c.PointsAvailable)
				assert.Equal(uint(0), c.PointsEarned)
			}
		} else {
			assert.Equal(exp.points, res.Test.PointsEarned)
			if exp.points != res.Test.PointsEarned {
				t.Log(res.Test.OutputJSON())
				t.FailNow()
			}

			assert.Equal(string(exp.result), string(res.Test.Result))
			for _, c := range res.Test.Commands {
				if cmd, ok2 := exp.cmdPoints[c.Id()]; !ok2 {
					assert.Equal(uint(0), c.PointsAvailable)
					assert.Equal(uint(0), c.PointsEarned)
				} else {
					assert.Equal(cmd.points, c.PointsEarned)
					assert.Equal(cmd.status, c.Status)
				}
			}
		}
	}

	env.manager.stop()

	assert.Equal(totalExpected, tg.EarnedPoints())

	return tg
}

func getPartialTestPoints() map[string]*expectedTestResults {
	testPoints := make(map[string]*expectedTestResults)

	// sem1
	testPoints["sync/sem1.t"] = &expectedTestResults{
		points: 10,
		result: TEST_RESULT_CORRECT,
	}

	// lt1
	testPoints["sync/lt1.t"] = &expectedTestResults{
		points: 10,
		result: TEST_RESULT_CORRECT,
	}

	// fail
	testPoints["sync/fail.t"] = &expectedTestResults{
		points: 10,
		result: TEST_RESULT_INCORRECT,
		cmdPoints: map[string]*expectedCmdResults{
			"sem1": &expectedCmdResults{
				status: COMMAND_STATUS_CORRECT,
				points: 10,
			},
			"panic": &expectedCmdResults{
				status: COMMAND_STATUS_INCORRECT,
				points: 0,
			},
			"lt1": &expectedCmdResults{
				status: COMMAND_STATUS_NONE,
				points: 0,
			},
			"cvt1": &expectedCmdResults{
				status: COMMAND_STATUS_NONE,
				points: 0,
			},
		},
	}

	return testPoints
}

func TestTargetScorePartial(t *testing.T) {
	// t.Parallel()
	testPoints := getPartialTestPoints()
	runTargetTest(t, testPoints, "partial")
}

func TestTargetScoreEntire(t *testing.T) {
	// t.Parallel()
	testPoints := make(map[string]*expectedTestResults)

	// sem1
	testPoints["sync/sem1.t"] = &expectedTestResults{
		points: 10,
		result: TEST_RESULT_CORRECT,
	}

	// lt1
	testPoints["sync/lt1.t"] = &expectedTestResults{
		points: 10,
		result: TEST_RESULT_CORRECT,
	}

	// fail
	testPoints["sync/fail.t"] = &expectedTestResults{
		points: 0,
		result: TEST_RESULT_INCORRECT,
		cmdPoints: map[string]*expectedCmdResults{
			"sem1": &expectedCmdResults{
				status: COMMAND_STATUS_CORRECT,
				points: 0,
			},
			"panic": &expectedCmdResults{
				status: COMMAND_STATUS_INCORRECT,
				points: 0,
			},
			"lt1": &expectedCmdResults{
				status: COMMAND_STATUS_NONE,
				points: 0,
			},
			"cvt1": &expectedCmdResults{
				status: COMMAND_STATUS_NONE,
				points: 0,
			},
		},
	}

	runTargetTest(t, testPoints, "entire")
}

func TestTargetScoreFull(t *testing.T) {
	t.Parallel()
	testPoints := make(map[string]*expectedTestResults)

	// sem1
	testPoints["sync/sem1.t"] = &expectedTestResults{
		points: 10,
		result: TEST_RESULT_CORRECT,
	}

	// lt1
	testPoints["sync/lt1.t"] = &expectedTestResults{
		points: 10,
		result: TEST_RESULT_CORRECT,
	}

	// multi
	testPoints["sync/multi.t"] = &expectedTestResults{
		points: 40,
		result: TEST_RESULT_CORRECT,
		cmdPoints: map[string]*expectedCmdResults{
			"sem1": &expectedCmdResults{
				status: COMMAND_STATUS_CORRECT,
				points: 0,
			},
			"lt1": &expectedCmdResults{
				status: COMMAND_STATUS_CORRECT,
				points: 0,
			},
		},
	}

	runTargetTest(t, testPoints, "full")
}
