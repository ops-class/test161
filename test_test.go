package test161

import (
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

func TestLoad(t *testing.T) {
	assert := assert.New(t)

	data := `---
name: boot
description: >
  Ensure that the kernel can boot.
tags: [basic,setup]
---
q`
	test, err := Load(([]byte)(data))
	assert.Nil(err)
	assert.Equal(test.Name, "boot")
	assert.NotNil(test.Description)
	assert.Equal(reflect.DeepEqual(test.Tags, []string{"basic", "setup"}), true)
	assert.Nil(test.Depends)

	data = `---
name: shell
depends:
- boot
---
$ /bin/true
`
	test, err = Load(([]byte)(data))
	assert.Nil(err)
	assert.Equal(test.Name, "shell")
	assert.Equal(test.Description, "")
	assert.Nil(test.Tags)
	assert.Equal(reflect.DeepEqual(test.Depends, []string{"boot"}), true)
}
