package test161

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestUsageStatsSubmission(t *testing.T) {
	t.Parallel()
	testPoints := getPartialTestPoints()

	targetName := "partial"

	start := time.Now()
	tg := runTargetTest(t, testPoints, targetName)
	end := time.Now()
	stats := NewTestGroupUsageStat([]string{"test161"}, targetName, tg, start, end)

	assert := assert.New(t)

	assert.Equal(1, len(stats.Users))
	if len(stats.Users) == 1 {
		assert.Equal("test161", stats.Users[0])
	}
	assert.Equal(Version.String(), stats.Test161Version)
	assert.Equal(CUR_USAGE_VERSION, stats.Version)

	assert.Equal(uint(60), stats.GroupInfo.PointsAvailable)
	assert.Equal(uint(30), stats.GroupInfo.Score)
	assert.Equal(targetName, stats.GroupInfo.TargetTagName)
	assert.Equal(start, stats.GroupInfo.SubmissionTime)
	assert.Equal(end, stats.GroupInfo.CompletionTime)
}
