package test161

import (
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

func TestLoad(t *testing.T) {
	assert := assert.New(t)

	test, err := LoadTest("./fixtures/tests/boot.yml")
	assert.Nil(err)
	assert.Equal(test.Name, "boot")
	assert.NotNil(test.Description)
	assert.Equal(reflect.DeepEqual(test.Tags, []string{"basic", "setup"}), true)
	assert.Nil(test.Depends)
	assert.Equal(test.Config.CPUs, (uint8)(4))
	assert.Equal(test.Config.Memory, (uint32)(16777216))

	test, err = LoadTest("./fixtures/tests/shell.yml")
	assert.Nil(err)
	assert.Equal(test.Name, "shell")
	assert.Equal(test.Description, "")
	assert.Nil(test.Tags)
	assert.Equal(reflect.DeepEqual(test.Depends, []string{"boot"}), true)
	assert.Equal(test.Config.Memory, (uint32)(16777216))
}

func TestRun(t *testing.T) {
	assert := assert.New(t)

	test, err := LoadTest("./fixtures/tests/boot.yml")
	assert.Nil(err)
	err = test.Run("./fixtures/ASST0/kernel", "", "")
}
