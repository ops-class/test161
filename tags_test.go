package test161

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTagDescriptionsLoad(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	text := `tags:
  - name: tag1
    desc: "This is desc1"
  - name: tag2
    desc: "This is desc2"
  - name: tag3
    desc: "This is desc3"
  - name: tag4
    desc: "This is desc4"
  - name: tag5
    desc: "This is desc5"
`
	all, err := TagDescriptionsFromString(text)
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	assert.Equal(5, len(all.Tags))
	if len(all.Tags) == 5 {
		for i, tag := range all.Tags {
			assert.Equal(fmt.Sprintf("tag%v", i+1), tag.Name)
			assert.Equal(fmt.Sprintf("This is desc%v", i+1), tag.Description)
		}
	}
}

func TestTagDescriptionsLoadFile(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	assert.Equal(5, len(defaultEnv.Tags))
	for i := 1; i <= 5; i++ {
		key := fmt.Sprintf("tag%v", i)
		tag, ok := defaultEnv.Tags[key]
		assert.True(ok)
		if ok {
			assert.Equal(key, tag.Name)
			assert.Equal(fmt.Sprintf("This is desc%v", i), tag.Description)
		}
	}
}
