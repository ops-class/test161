package test161

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestBuildFull(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	conf := &BuildConf{
		Repo:     "git@gitlab.ops-class.org:staff/sol1.git",
		CommitID: "HEAD",
		KConfig:  "ASST1",
		//RequiredCommit: "db6d3d219d53a292b96e8529649757bb257e8785",
		Overlay: "asst1",
	}

	env := defaultEnv.CopyEnvironment()
	env.RootDir = "./fixtures/root"

	test, err := conf.ToBuildTest(env)
	assert.Nil(err)
	assert.NotNil(test)

	if test == nil {
		t.Log(err)
		t.FailNow()
	}

	_, err = test.Run(env)
	assert.Nil(err)

	t.Log(test.OutputJSON())

	for k, v := range env.keyMap {
		t.Log(k, v)
	}

}

type confDetail struct {
	repo      string
	commit    string
	config    string
	reqCommit string
}

func TestBuildFailures(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	configs := []confDetail{
		confDetail{"https://notgithub.com/ops-class/os161111.git", "HEAD", "DUMBVM", ""},
		confDetail{"https://github.com/ops-class/os161.git", "aaaaaaaaaaa111111112222", "FOO", ""},
		confDetail{"https://github.com/ops-class/os161.git", "HEAD", "FOO", ""},
		confDetail{"https://github.com/ops-class/os161.git", "HEAD", "DUMBVM", "notavalidcommitit"},
	}

	for _, c := range configs {

		conf := &BuildConf{
			Repo:           c.repo,
			CommitID:       c.commit,
			KConfig:        c.config,
			RequiredCommit: c.reqCommit,
		}

		test, err := conf.ToBuildTest(defaultEnv)
		assert.NotNil(test)

		res, err := test.Run(defaultEnv)
		assert.NotNil(err)
		assert.Nil(res)
	}
}
