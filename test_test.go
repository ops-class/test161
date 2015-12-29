package test161

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"os"
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

	for _, test := range []string{"boot", "shell", "parallelvm"} {
		test, err := LoadTest(fmt.Sprintf("./fixtures/tests/%v.yml", test))
		assert.Nil(err)
		conf, err := test.PrintConf()
		assert.Nil(err)
		assert.NotNil(conf)
		t.Log(conf)
	}
}

func TestRunBoot(t *testing.T) {
	assert := assert.New(t)

	test, err := LoadTest("./fixtures/tests/boot.yml")
	assert.Nil(err)
	err = test.Run("./fixtures/ASST0/kernel", "", "")
	assert.Nil(err)
}
