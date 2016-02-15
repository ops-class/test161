package test161

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSubmissionRun(t *testing.T) {

	t.Parallel()

	assert := assert.New(t)

	var err error

	req := &SubmissionRequest{
		Target: "asst1",
		Users: []*SubmissionUserInfo{
			&SubmissionUserInfo{
				Email: "t1@xcv58.com",
				Token: "ATamoCT7DdeNdnErQ",
			},
		},
		Repository: "git@gitlab.ops-class.org:staff/sol1.git",
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

	env.manager.stop()
}
