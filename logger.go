package test161

import (
	"github.com/gchallen/expect"
	"regexp"
	"time"
)

// Recv processes new sys161 output and restarts the progress timer
func (t *Test) Recv(receivedTime time.Time, received []byte) {
	if !t.statStarted {
		go t.getStats()
		t.statStarted = true
	}

	t.commandLock.Lock()
	defer t.commandLock.Unlock()
	t.progressTime = float64(t.SimTime)
	for _, b := range received {
		if t.currentOutput.Delta == 0.0 {
			t.currentOutput.Delta = t.getDelta()
		}
		t.currentOutput.Buffer.WriteByte(b)
		if b == '\n' {
			t.currentOutput.Line = t.currentOutput.Buffer.String()
			t.command.Output = append(t.command.Output, t.currentOutput)
			t.currentOutput = OutputLine{}
			continue
		}
	}
}

// Unused parts of the expect.Logger interface
func (t *Test) Send(time.Time, []byte)                      {}
func (t *Test) SendMasked(time.Time, []byte)                {}
func (t *Test) RecvNet(time.Time, []byte)                   {}
func (t *Test) RecvEOF(time.Time)                           {}
func (t *Test) ExpectCall(time.Time, *regexp.Regexp)        {}
func (t *Test) ExpectReturn(time.Time, expect.Match, error) {}
func (t *Test) Close(time.Time)                             {}
