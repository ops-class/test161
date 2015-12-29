package test161

import (
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

func TestLoad(t *testing.T) {
	assert := assert.New(t)

	test, err := LoadTest("./fixtures/boot.yml")
	assert.Nil(err)
	assert.Equal(test.Name, "boot")
	assert.NotNil(test.Description)
	assert.Equal(reflect.DeepEqual(test.Tags, []string{"basic", "setup"}), true)
	assert.Nil(test.Depends)

	test, err = LoadTest("./fixtures/shell.yml")
	assert.Nil(err)
	assert.Equal(test.Name, "shell")
	assert.Equal(test.Description, "")
	assert.Nil(test.Tags)
	assert.Equal(reflect.DeepEqual(test.Depends, []string{"boot"}), true)
}
