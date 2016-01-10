package test161

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestLoggerTimerKill(t *testing.T) {
	assert := assert.New(t)

	test, err := TestFromString("$ /testbin/spinner 16")
	assert.Nil(err)

	go func() {
		err = test.Run("./fixtures/", "")
	}()
	time.Sleep(time.Second)
	assert.Nil(err)
	assert.Equal(test.Status, "")
	test.TimerKill()
	time.Sleep(time.Second)
	assert.Nil(err)
	assert.Equal(test.Status, "timeout")

	t.Log(test.OutputJSON())
	t.Log(test.OutputString())
}
