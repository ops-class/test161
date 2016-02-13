package test161

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestBuildFull(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	conf := &BuildConf{
		Repo:           "https://github.com/ops-class/os161.git",
		CommitID:       "e9e9b91904d5b098e1c69cd4ed23dfd65f6f212d",
		KConfig:        "DUMBVM",
		CacheDir:       "",
		RequiredCommit: "db6d3d219d53a292b96e8529649757bb257e8785",
	}

	test, err := conf.ToBuildTest()
	assert.Nil(err)
	assert.NotNil(test)

	if test == nil {
		t.Log(err)
		t.FailNow()
	}

	_, err = test.Run(defaultEnv)
	assert.Nil(err)

	t.Log(test.OutputJSON())
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

		test, err := conf.ToBuildTest()
		assert.NotNil(test)

		res, err := test.Run(defaultEnv)
		assert.NotNil(err)
		assert.Nil(res)
	}
}
