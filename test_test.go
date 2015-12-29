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
	assert.Equal(test.Conf.Disk1.Sectors, "10240")
	assert.Equal(test.Conf.Disk1.RPM, "7200")

	test, err = LoadTest("./fixtures/tests/shell.yml")
	assert.Nil(err)
	assert.Equal(test.Name, "shell")
	assert.Equal(test.Description, "")
	assert.Nil(test.Tags)
	assert.Equal(reflect.DeepEqual(test.Depends, []string{"boot"}), true)
	assert.Equal(test.Conf.CPUs, "1")
	assert.Equal(test.Conf.RAM, "16777216")

	test, err = LoadTest("./fixtures/tests/parallelvm.yml")
	assert.Nil(err)
	assert.Equal(test.Name, "parallelvm")
	assert.Equal(test.Description, "")
	assert.Nil(test.Tags)
	assert.Equal(reflect.DeepEqual(test.Depends, []string{"shell"}), true)
	assert.Equal(test.Conf.CPUs, "1")
	assert.Equal(test.Conf.RAM, "2097152")
	assert.Equal(test.Conf.Disk2.Sectors, "10240")
	assert.Equal(test.Conf.Disk2.RPM, "14400")
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
