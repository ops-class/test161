package test161

import (
	"encoding/json"
	"fmt"
	"github.com/gchallen/expect"
	"regexp"
	"time"
)

func (t *Test) Recv(receivedTime time.Time, received []byte) {
	t.progressTimer.Reset(time.Duration(t.MonitorConf.Timeouts.Progress) * time.Second)

	t.commandLock.Lock()
	defer t.commandLock.Unlock()
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

func (t *Test) OutputJSON() (string, error) {
	outputBytes, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return "", err
	}
	return string(outputBytes), nil
}

func (t *Test) TimerKill() {
	t.commandLock.Lock()
	if t.Status == "" {
		t.Status = "timeout"
		t.ShutdownMessage = fmt.Sprintf("no progress for %d s", t.MonitorConf.Timeouts.Progress)
		t.sys161.Killer()
	}
	t.commandLock.Unlock()
}
