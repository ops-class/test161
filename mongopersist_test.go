package test161

import (
	"github.com/stretchr/testify/assert"
	"gopkg.in/mgo.v2"
	"testing"
	"time"
)

// MongoDB test connection
var mongoTestDialInfo = &mgo.DialInfo{
	Addrs:    []string{"localhost:27017"},
	Timeout:  60 * time.Second,
	Database: "test",
	Username: "",
	Password: "",
}

func TestMongoBoot(t *testing.T) {
	t.Parallel()

	if !testFlagDB {
		t.Skip("Skipping MongoDB Test")
	}

	assert := assert.New(t)

	mongo, err := NewMongoPersistence(mongoTestDialInfo)
	assert.Nil(err)
	assert.NotNil(mongo)

	if err != nil {
		t.FailNow()
	}
	defer mongo.Close()

	test, err := TestFromString("q")
	assert.Nil(err)
	assert.Nil(test.MergeConf(TEST_DEFAULTS))

	env := defaultEnv.CopyEnvironment()
	env.Persistence = mongo
	env.RootDir = defaultEnv.RootDir

	assert.Nil(env.Persistence.Notify(test, MSG_PERSIST_CREATE, 0))
	assert.Nil(test.Run(env))
}
