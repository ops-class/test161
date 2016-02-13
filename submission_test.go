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
		Target:     "asst1",
		Users:      []string{"foo@bar.com"},
		Repository: "git@gitlab.ops-class.org:staff/sol1.git",
		CommitID:   "HEAD",
	}

	env := defaultEnv.CopyEnvironment()

	if testFlagDB {
		mongo, err := NewMongoPersistence(mongoTestDialInfo)
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

	env.manager.start()

	err = s.Run()
	assert.Nil(err)
	assert.Equal(uint(50), s.Score)

	env.manager.stop()
}
