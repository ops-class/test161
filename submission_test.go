package test161

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSubmissionObject(t *testing.T) {
	assert := assert.New(t)

	req := &SubmissionRequest{
		Target:     "full",
		Users:      []string{"foo@bar.com"},
		Repository: "git@gitlab.ops-class.org:staff/sol3.git",
		CommitID:   "HEAD",
	}

	env := defaultEnv.CopyEnvironment()
	mongo, err := NewMongoPersistence()
	assert.Nil(err)
	assert.NotNil(mongo)

	env.Persistence = mongo
	defer mongo.Close()

	env.manager = newManager()

	s, errs := NewSubmission(req, env)
	assert.Equal(0, len(errs))
	assert.NotNil(s)

	env.manager.start()
	err = s.Run()
	assert.Nil(err)
	assert.Equal(uint(60), s.Score)
}
