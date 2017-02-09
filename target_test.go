package test161

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	if len(target.MetaName) == 0 {
		assert.Equal(target.Points, tg.TotalPoints())
	} else {
		// For metatarget subtargets, we need to sum the points for
		// all targets that will be run.
		totalPoints := target.Points
		for _, other := range target.previousSubTargets {
			totalPoints += other.Points
		}
		assert.Equal(totalPoints, tg.TotalPoints())
	}

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

func TestMetaTargetLoad(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	text := `---
name: asst0
points: 100
type: asst
sub_target_names: [asst0.1, asst0.2, asst0.3]
`
	target, err := TargetFromString(text)
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	assert.Equal(3, len(target.SubTargetNames))
	if len(target.Tests) == 3 {
		assert.Equal("asst0.1", target.SubTargetNames[0])
		assert.Equal("asst0.2", target.SubTargetNames[1])
		assert.Equal("asst0.3", target.SubTargetNames[2])
	}

	tg, errs := target.Instance(defaultEnv)
	assert.Equal(1, len(errs))
	assert.Nil(tg)
}

func TestMetaTargetLoadFile(t *testing.T) {
	assert := assert.New(t)

	target, ok := defaultEnv.Targets["metatest"]
	t.Log(defaultEnv.Targets)
	assert.True(ok)
	if !ok {
		t.FailNow()
	}

	assert.Equal(2, len(target.SubTargetNames))
	if len(target.SubTargetNames) != 2 {
		t.FailNow()
	}
	assert.Equal("meta.1", target.SubTargetNames[0])
	assert.Equal("meta.2", target.SubTargetNames[1])
	assert.True(target.IsMetaTarget)
}

func TestMetaTargetLoadSubTarget(t *testing.T) {
	assert := assert.New(t)

	target, ok := defaultEnv.Targets["meta.2"]
	assert.True(ok)
	if !ok {
		t.FailNow()
	}

	assert.NotNil(target.metaTarget)
	if target.metaTarget == nil {
		t.FailNow()
	}
	assert.Equal(1, len(target.previousSubTargets))
	assert.Equal("metatest", target.MetaName)
	assert.Equal("metatest", target.metaTarget.Name)

	assert.False(target.IsMetaTarget)

	tg, errs := target.Instance(defaultEnv)
	t.Log(errs)
	assert.Equal(0, len(errs))
	assert.NotNil(tg)

	if tg == nil {
		t.FailNow()
	}
	for _, test := range tg.Tests {
		assert.True(len(test.requiredBy) > 0)
		t.Log(test.DependencyID, test.requiredBy)
	}
}

func TestMetaTargetRun(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	testPoints := make(map[string]*expectedTestResults)

	// sem1
	testPoints["sync/sem1.t"] = &expectedTestResults{
		points: 25,
		result: TEST_RESULT_CORRECT,
	}

	// lt1
	testPoints["sync/lt1.t"] = &expectedTestResults{
		points: 75,
		result: TEST_RESULT_CORRECT,
	}

	tg := runTargetTest(t, testPoints, "meta.2")

	count := 0
	for _, test := range tg.Tests {
		if test.DependencyID == "sync/sem1.t" {
			assert.Equal("meta.1", test.TargetName)
			count += 1
		} else if test.DependencyID == "sync/lt1.t" {
			assert.Equal("meta.2", test.TargetName)
			count += 1
		}
	}

	assert.Equal(2, count)
}

func TestMetaTargetInconsistent(t *testing.T) {
	// Not parallel
	require := require.New(t)

	target := defaultEnv.Targets["metatest"]
	require.NotNil(target)

	err := target.initAsMetaTarget(defaultEnv)
	require.Nil(err)

	// Point total
	target.Points += 1
	err = target.initAsMetaTarget(defaultEnv)
	require.NotNil(err)
	target.Points -= 1
	err = target.initAsMetaTarget(defaultEnv)
	require.Nil(err)

	subtarget := defaultEnv.Targets["meta.2"]
	require.NotNil(subtarget)

	// Different userland requirement
	subtarget.RequiresUserland = true
	require.NotNil(target.initAsMetaTarget(defaultEnv))
	subtarget.RequiresUserland = false
	require.Nil(target.initAsMetaTarget(defaultEnv))

	// Different configs
	prev := subtarget.KConfig
	subtarget.KConfig += "111"
	require.NotNil(target.initAsMetaTarget(defaultEnv))
	subtarget.KConfig = prev
	require.Nil(target.initAsMetaTarget(defaultEnv))

	// Different types
	subtarget.Type = "perf"
	require.NotNil(target.initAsMetaTarget(defaultEnv))
	subtarget.Type = "asst"
	require.Nil(target.initAsMetaTarget(defaultEnv))

}
