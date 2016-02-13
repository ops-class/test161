package test161

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

/*
func TestBuildGitOnly(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	conf, err := NewBuildConf("", "", "")
	assert.Nil(err)
	assert.NotNil(conf)
	if conf == nil {
		return
	}

	defer conf.CleanUp()

	conf.Repo = "git@gitlab.ops-class.org:staff/sol3.git"
	conf.CommitID = "1b17c415eec4eb3889f177bb99ed714f706352a7"
	conf.Config = "SOL3"

	o, e := conf.getSources()
	assert.Nil(e)
	t.Log(e)
	t.Log(o)
}
*/

func TestBuildFull(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	conf := &BuildConf{}
	conf.Repo = "git@gitlab.ops-class.org:staff/sol3.git"
	conf.CommitID = "HEAD"
	conf.KConfig = "SOL3"
	conf.CacheDir = "/home/shaseley/cache"
	conf.RequiredCommit = "29b635f4b8393fda987244b45ab0e32a61ea5dcb"

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

/*
type confDetail struct {
	repo   string
	commit string
	config string
}

func TestBuildFailures(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	configs := []confDetail{
		confDetail{"git@gitlab.ops-class.org:staff/sol3.git", "HEAD", ""},
		confDetail{"git@gitlab.ops-class.org:staff/sol3.git", "aaaaaaaaaaa111111112222", "SOL3"},
		confDetail{"git@gitlab.ops-classss.org:staff/sol3.git", "HEAD", "SOL3"},
		confDetail{"git@gitlab.ops-class.org:staff/sol50.git", "aaaaaaaaaaa111111112222", "SOL3"},
	}

	for _, c := range configs {
		conf, err := NewBuildConf(c.repo, c.commit, c.config)
		assert.Nil(err)
		if conf == nil {
			t.Log(c)
			t.FailNow()
		}

		o, e := conf.GitAndBuild()
		assert.NotNil(e)
		if e == nil {
			t.Log(c)
		}
		t.Log(o)
		conf.CleanUp()
	}
}
*/
