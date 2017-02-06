package test161

import (
	"encoding/json"
	"github.com/satori/go.uuid"
	"time"
)

const CUR_USAGE_VERSION = 1

type UsageStat struct {
	ID             string     `bson:"_id"`
	Users          []string   `bson:"users" json:"users"`
	Timestamp      time.Time  `bson:"timestamp" json:"timestamp"`
	Version        int        `bson:"version" json:"version"`
	Test161Version string     `bson:"test161_version" json:"test161_version"`
	GroupInfo      *GroupStat `bson:"group_info" json:"group_info"`
}

type GroupStat struct {
	TargetTagName   string      `bson:"target_tag_name" json:"target_tag_name"`
	PointsAvailable uint        `bson:"max_score" json:"max_score"`
	Status          string      `bson:"status" json:"status"`
	Score           uint        `bson:"score" json:"score"`
	Tests           []*TestStat `bson:"tests" json:"tests"`
	Errors          []string    `bson:"errors" json:"errors"`
	SubmissionTime  time.Time   `bson:"submission_time" json:"submission_time"`
	CompletionTime  time.Time   `bson:"completion_time" json:"completion_time"`
}

type TestStat struct {
	Name            string     `json:"name" bson:"name"`
	Result          TestResult `json:"result" bson:"result"`
	PointsAvailable uint       `json:"points_avail" bson:"points_avail"`
	PointsEarned    uint       `json:"points_earned" bson:"points_earned"`
	MemLeakBytes    int        `json:"mem_leak_bytes" bson:"mem_leak_bytes"`
	MemLeakPoints   uint       `json:"mem_leak_points" bson:"mem_leak_points"`
	MemLeakDeducted uint       `json:"mem_leak_deducted" bson:"mem_leak_deducted"`
}

func (stat *UsageStat) JSON() (string, error) {
	if data, err := json.Marshal(stat); err != nil {
		return "", err
	} else {
		return string(data), nil
	}
}

func (stat *UsageStat) Persist(env *TestEnvironment) error {
	return env.Persistence.Notify(stat, MSG_PERSIST_CREATE, 0)
}

func newUsageStat(users []string) *UsageStat {
	stat := &UsageStat{
		ID:             uuid.NewV4().String(),
		Users:          users,
		Timestamp:      time.Now(),
		Test161Version: Version.String(),
		Version:        CUR_USAGE_VERSION,
	}
	return stat
}

func NewTestGroupUsageStat(users []string, targetOrTag string, tg *TestGroup,
	startTime, endTime time.Time) *UsageStat {

	stat := newUsageStat(users)
	stat.GroupInfo = &GroupStat{
		TargetTagName:   targetOrTag,
		PointsAvailable: 0,
		Status:          "",
		Score:           0,
		Errors:          []string{},
		SubmissionTime:  startTime,
		CompletionTime:  endTime,
	}
	stat.GroupInfo.Tests = testStatsFromGroup(tg)
	for _, t := range stat.GroupInfo.Tests {
		stat.GroupInfo.PointsAvailable += t.PointsAvailable
		stat.GroupInfo.Score += t.PointsEarned
	}

	return stat
}

func testStatsFromGroup(group *TestGroup) []*TestStat {
	tests := make([]*TestStat, 0, len(group.Tests))
	for _, test := range group.Tests {
		tests = append(tests, newTestStat(test))
	}
	return tests
}

func newTestStat(t *Test) *TestStat {
	stat := &TestStat{
		Name:            t.Name,
		Result:          t.Result,
		PointsAvailable: t.PointsAvailable,
		PointsEarned:    t.PointsEarned,
		MemLeakBytes:    t.MemLeakBytes,
		MemLeakPoints:   t.MemLeakPoints,
		MemLeakDeducted: t.MemLeakDeducted,
	}
	return stat
}
