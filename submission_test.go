package test161

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSubmissionRun(t *testing.T) {

	t.Parallel()

	assert := assert.New(t)

	var err error

	req := &SubmissionRequest{
		Target: "simple",
		Users: []*SubmissionUserInfo{
			&SubmissionUserInfo{
				Email: testStudent.Email,
				Token: testStudent.Token,
			},
		},
		Repository: "git@github.com:ops-class/os161.git",
		CommitID:   "HEAD",
	}

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

	env.manager.stop()
}

func retrieveTestStudent(persist PersistenceManager) []*Student {
	students := []*Student{}
	request := map[string]interface{}{
		"email": testStudent.Email,
		"token": testStudent.Token,
	}

	persist.Retrieve(PERSIST_TYPE_STUDENTS, request, &students)

	return students
}
