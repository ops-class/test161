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

func TestMongoGetUser(t *testing.T) {
	t.Parallel()

	if !testFlagDB {
		t.Skip("Skipping MongoDB Test")
	}

	assert := assert.New(t)

	// MongoDB test connection
	var di = &mgo.DialInfo{
		Addrs:    []string{"localhost:27017"},
		Timeout:  60 * time.Second,
		Database: "test161",
		Username: "",
		Password: "",
	}

	mongo, err := NewMongoPersistence(di)
	assert.Nil(err)
	assert.NotNil(mongo)

	if err != nil {
		t.FailNow()
	}
	defer mongo.Close()

	staff := "services.auth0.user_metadata.staff"
	who := map[string]interface{}{"services.auth0.email": "admin@ops-class.org"}
	filter := map[string]interface{}{staff: 1}
	res := make([]interface{}, 0)

	err = mongo.Retrieve(PERSIST_TYPE_USERS, who, filter, &res)
	assert.Nil(err)
	assert.True(len(res) > 0)
	t.Log(res)
}
