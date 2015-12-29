package test161

import (
	"github.com/stretchr/testify/assert"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	rand.Seed(time.Now().UTC().UnixNano())
	os.Exit(m.Run())
}

func TestLoad(t *testing.T) {
	assert := assert.New(t)

	test, err := LoadTest("./fixtures/tests/boot.yml")
	assert.Nil(err)
	assert.Equal(test.Name, "boot")
	assert.NotNil(test.Description)
	assert.Equal(reflect.DeepEqual(test.Tags, []string{"basic", "setup"}), true)
	assert.Nil(test.Depends)
	assert.Equal(test.Conf.CPUs, "4")
	assert.Equal(test.Conf.RAM, "16777216")

	test, err = LoadTest("./fixtures/tests/shell.yml")
	assert.Nil(err)
	assert.Equal(test.Name, "shell")
	assert.Equal(test.Description, "")
	assert.Nil(test.Tags)
	assert.Equal(reflect.DeepEqual(test.Depends, []string{"boot"}), true)
	assert.Equal(test.Conf.CPUs, "1")
	assert.Equal(test.Conf.RAM, "16777216")
}

func TestPrintConf(t *testing.T) {
	assert := assert.New(t)

	test, err := LoadTest("./fixtures/tests/boot.yml")
	assert.Nil(err)
	err = test.PrintConf()
	assert.Nil(err)
}

func TestRun(t *testing.T) {
	assert := assert.New(t)

	test, err := LoadTest("./fixtures/tests/boot.yml")
	assert.Nil(err)
	err = test.Run("./fixtures/ASST0/kernel", "", "")
	assert.Nil(err)
}
