package test161

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func genTestSubmissionRequest(targetName string) *SubmissionRequest {
	req := &SubmissionRequest{
		Target: targetName,
		Users: []*SubmissionUserInfo{
			&SubmissionUserInfo{
				Email: testStudent.Email,
				Token: testStudent.Token,
			},
		},
		Repository:    "git@github.com:ops-class/os161.git",
		CommitID:      "HEAD",
		ClientVersion: Version,
	}
	return req
}

func TestSubmissionRun(t *testing.T) {
	assert := assert.New(t)

	var err error

	req := genTestSubmissionRequest("simple")
	env := defaultEnv.CopyEnvironment()

	if testFlagDB {
		dialInfo := *mongoTestDialInfo
		dialInfo.Database = "test161"
		mongo, err := NewMongoPersistence(&dialInfo)
		assert.Nil(err)
		assert.NotNil(mongo)
		if err != nil {
			t.FailNow()
		}
		env.Persistence = mongo
		defer mongo.Close()
	} else {
		env.Persistence = &TestingPersistence{}
	}

	env.manager = newManager()

	s, errs := NewSubmission(req, env)
	assert.Equal(0, len(errs))
	assert.NotNil(s)

	if s == nil || len(errs) > 0 {
		t.Log(errs)
		t.FailNow()
	}

	env.manager.start()

	err = s.Run()
	assert.Nil(err)

	assert.Equal(uint(50), s.Score)
	assert.True(len(s.OverlayCommitID) > 0)
	assert.True(isHexString(s.OverlayCommitID))
	assert.True(s.IsStaff)

	students := retrieveTestStudent(env.Persistence)
	assert.Equal(1, len(students))
	if len(students) != 1 {
		t.FailNow()
	}

	// The submission makes a copy of of env with a fresh keymap, so use that
	assert.True(len(s.Env.keyMap) > 0)
	t.Log(s.Env.keyMap)

	outputBytes, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Log(err)
		t.FailNow()
	}
	t.Log(string(outputBytes))

	stat := students[0].getStat("simple")
	assert.NotNil(stat)
	assert.Equal(uint(50), stat.MaxScore)
	assert.Equal(uint(50), stat.HighScore)

	// Check test ids
	assert.True(len(s.ID) > 0)
	assert.Equal(s.ID, s.BuildTest.SubmissionID)
	for _, test := range s.Tests.Tests {
		assert.Equal(s.ID, test.SubmissionID)
	}

	env.manager.stop()
}

func TestMetaTargetSubmissionRun(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	var err error

	req := genTestSubmissionRequest("meta.2")
	env := defaultEnv.CopyEnvironment()

	if testFlagDB {
		dialInfo := *mongoTestDialInfo
		dialInfo.Database = "test161"
		mongo, err := NewMongoPersistence(&dialInfo)
		assert.Nil(err)
		assert.NotNil(mongo)
		if err != nil {
			t.FailNow()
		}
		env.Persistence = mongo
		defer mongo.Close()
	} else {
		env.Persistence = &TestingPersistence{}
	}

	env.manager = newManager()

	s, errs := NewSubmission(req, env)
	assert.Equal(0, len(errs))
	assert.NotNil(s)

	if s == nil || len(errs) > 0 {
		t.Log(errs)
		t.FailNow()
	}

	// Test target name flip
	assert.Equal("metatest", s.TargetName)
	assert.Equal("meta.2", s.SubmittedTargetName)
	assert.Equal(uint(100), s.PointsAvailable)

	env.manager.start()

	// Note: lt1 will fail when running this from the base sources.
	err = s.Run()
	assert.Nil(err)

	assert.Equal(uint(25), s.Score)
	sub, ok := s.subSubmissions["meta.1"]
	require.True(ok)
	assert.Equal(uint(25), sub.Score)

	sub, ok = s.subSubmissions["meta.2"]
	require.True(ok)
	assert.Equal(uint(0), sub.Score)

	require.Equal(2, len(s.SubSubmissionIDs))

	env.manager.stop()
}

func TestSubmissionMetaSplit(t *testing.T) {
	require := require.New(t)

	req := genTestSubmissionRequest("meta.2")

	s, errs := NewSubmission(req, defaultEnv)
	require.NotNil(s)
	require.Equal(0, len(errs))

	splits := s.split()

	// Test target name flip
	require.Equal("metatest", s.TargetName)
	require.Equal("meta.2", s.SubmittedTargetName)
	require.Equal(uint(100), s.PointsAvailable)

	// Should have one for orig, and one for meta.1
	require.Equal(2, len(splits))
	sMap := make(map[string]*Submission)
	for _, sub := range splits {
		sMap[sub.TargetName] = sub
	}

	sub, ok := sMap["meta.1"]
	require.True(ok)
	require.Equal(uint(25), sub.PointsAvailable)
	require.Equal(s.ID, sub.OrigSubmissionID)
	require.NotEqual(s.ID, sub.ID)

	sub, ok = sMap["meta.2"]
	require.True(ok)
	require.Equal(uint(75), sub.PointsAvailable)
	require.Equal(s.ID, sub.OrigSubmissionID)
	require.NotEqual(s.ID, sub.ID)
}

func retrieveTestStudent(persist PersistenceManager) []*Student {
	students := []*Student{}
	request := map[string]interface{}{
		"email": testStudent.Email,
		"token": testStudent.Token,
	}

	persist.Retrieve(PERSIST_TYPE_STUDENTS, request, nil, &students)

	return students
}

func TestMetaTargetSubmissionRunMeta(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	var err error

	req := genTestSubmissionRequest("metatest")
	env := defaultEnv.CopyEnvironment()

	if testFlagDB {
		dialInfo := *mongoTestDialInfo
		dialInfo.Database = "test161"
		mongo, err := NewMongoPersistence(&dialInfo)
		assert.Nil(err)
		assert.NotNil(mongo)
		if err != nil {
			t.FailNow()
		}
		env.Persistence = mongo
		defer mongo.Close()
	} else {
		env.Persistence = &TestingPersistence{}
	}

	env.manager = newManager()

	s, errs := NewSubmission(req, env)
	assert.Equal(0, len(errs))
	assert.NotNil(s)

	if s == nil || len(errs) > 0 {
		t.Log(errs)
		t.FailNow()
	}

	// Test target name flip
	assert.Equal("metatest", s.TargetName)
	assert.Equal("metatest", s.SubmittedTargetName)
	assert.Equal(uint(100), s.PointsAvailable)

	env.manager.start()

	// Note: lt1 will fail when running this from the base sources.
	err = s.Run()
	assert.Nil(err)

	assert.Equal(uint(25), s.Score)

	sub, ok := s.subSubmissions["meta.1"]
	require.True(ok)
	assert.Equal(uint(25), sub.Score)

	sub, ok = s.subSubmissions["meta.2"]
	require.True(ok)
	assert.Equal(uint(0), sub.Score)

	require.Equal(2, len(s.SubSubmissionIDs))

	env.manager.stop()
}
