package test161

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gchallen/expect"
	"github.com/termie/go-shutil"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"
)

const KERNEL_PROMPT = `OS/161 kernel [? for menu]: `
const SHELL_PROMPT = `OS/161$ `

type Command struct {
	ID           uint         `json:"-"`
	Env          string       `json:"env"`
	Input        InputLine    `json:"input"`
	Output       []OutputLine `json:"output"`
	SummaryStats Stat         `json:"summarystats"`
	AllStats     []Stat       `json:"stats"`
}

type InputLine struct {
	Delta TimeDelta `json:"delta"`
	Line  string    `json:"line"`
}

type OutputLine struct {
	Delta  TimeDelta    `json:"delta"`
	Buffer bytes.Buffer `json:"-"`
	Line   string       `json:"line"`
}

type TimeDelta float64

func (t TimeDelta) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%.6f", t)), nil
}

func (t *Test) getDelta() TimeDelta {
	return TimeDelta(float64(time.Now().UnixNano()-t.startTime) / float64(1000*1000*1000))
}

func (t *Test) Run(root string, tempRoot string) (err error) {

	tempRoot, err = ioutil.TempDir(tempRoot, "test161")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempRoot)
	tempDir := path.Join(tempRoot, "root")

	if root != "" {
		err = shutil.CopyTree(root, tempDir, nil)
		if err != nil {
			return err
		}
	}

	kernelTarget := path.Join(tempDir, "kernel")
	if _, err := os.Stat(kernelTarget); os.IsNotExist(err) {
		return err
	}

	confTarget := path.Join(tempDir, "sys161.conf")
	t.ConfString, err = t.PrintConf()
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(confTarget, []byte(t.ConfString), 0440)
	if err != nil {
		return err
	}
	if _, err := os.Stat(confTarget); os.IsNotExist(err) {
		return err
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return err
	}
	defer os.Chdir(currentDir)
	os.Chdir(tempDir)

	t.statCond = &sync.Cond{L: &sync.Mutex{}}
	t.commandLock = &sync.Mutex{}

	if t.Conf.Disk1.Sectors != "" {
		err = exec.Command("disk161", "create", t.Conf.Disk1.File, t.Conf.Disk1.Bytes).Run()
		if err != nil {
			return err
		}
	}
	if t.Conf.Disk2.Sectors != "" {
		err = exec.Command("disk161", "create", t.Conf.Disk2.File, t.Conf.Disk2.Bytes).Run()
		if err != nil {
			return err
		}
	}

	t.progressTimer =
		time.AfterFunc(time.Duration(t.MonitorConf.Timeouts.Progress)*time.Second, t.TimerKill)
	t.sys161, err = expect.Spawn("sys161", "-X", "kernel")
	t.startTime = time.Now().UnixNano()
	if err != nil {
		return err
	}
	defer t.sys161.Close()

	t.command = &Command{
		Input: InputLine{Delta: t.getDelta(), Line: "boot"},
	}
	t.sys161.SetLogger(t)
	t.sys161.SetTimeout(time.Duration(t.MonitorConf.Timeouts.Prompt) * time.Second)

	statConn, err := net.Dial("unix", path.Join(tempDir, ".sockets/meter"))
	if err != nil {
		return err
	}

	go t.getStats(statConn)

	prompts := regexp.MustCompile(fmt.Sprintf("(%s|%s)", regexp.QuoteMeta(KERNEL_PROMPT), regexp.QuoteMeta(SHELL_PROMPT)))

	match, err := t.sys161.ExpectRegexp(prompts)
	prompt := match.Groups[0]
	if err != nil {
		return err
	}
	if prompt != KERNEL_PROMPT {
		return errors.New(fmt.Sprintf("test161: expected kernel prompt, got %s", prompt))
	}
	currentEnv := "kernel"

	commands := strings.Split(strings.TrimSpace(t.Content), "\n")

	i := 0
	j := uint(0)

	var statError error
	for {
		var command string
		if i < len(commands) {
			command = strings.TrimSpace(commands[i])
			if command == "" {
				return errors.New("test161: found empty command")
			}
			if string(command[0]) == "$" && currentEnv == "kernel" {
				command = "s"
			} else if string(command[0]) != "$" && currentEnv == "shell" {
				command = "exit"
			} else {
				if string(command[0]) == "$" {
					command = command[1:]
				}
				i += 1
			}
		} else {
			if currentEnv == "kernel" {
				command = "q"
			} else if currentEnv == "shell" {
				command = "exit"
			}
		}
		t.statCond.L.Lock()
		if t.statActive {
			t.statCond.Wait()
		}
		statError = t.statError
		t.statCond.L.Unlock()
		if statError != nil {
			return statError
		}
		t.commandLock.Lock()
		if t.currentOutput.Delta != 0.0 {
			t.currentOutput.Line = t.currentOutput.Buffer.String()
			t.command.Output = append(t.command.Output, t.currentOutput)
		}
		t.currentOutput = OutputLine{}
		t.Commands = append(t.Commands, *t.command)
		if command != "" {
			t.command = &Command{
				ID:    j,
				Env:   currentEnv,
				Input: InputLine{Delta: t.getDelta(), Line: command},
			}
			j += 1
		}
		t.commandLock.Unlock()

		if command == "" {
			break
		}
		err = t.sys161.SendLn(command)
		if err != nil {
			return err
		}
		if command == "q" {
			currentEnv = ""
			t.commandLock.Lock()
			t.sys161.ExpectEOF()
			t.Status = "shutdown"
			t.RunTime = t.getDelta()
			t.commandLock.Unlock()
			continue
		}
		match, err := t.sys161.ExpectRegexp(prompts)

		// 10 Jan 2016 : GWA : Wait again for the stat signal so that we have
		// aligned stats at finish. This slows down testing somewhat, and it's not
		// perfectly accurate, but stats come along fairly rapidly and there
		// shouldn't be too many cycles added by waiting at the menu.

		t.statCond.L.Lock()
		if t.statActive {
			t.statCond.Wait()
		}
		statError = t.statError
		t.statCond.L.Unlock()
		if statError != nil {
			return statError
		}

		if err == expect.ErrTimeout {
			currentEnv = ""
			i = len(commands)
			t.commandLock.Lock()
			t.Status = "timeout"
			t.ShutdownMessage = fmt.Sprintf("no prompt for %d s", t.MonitorConf.Timeouts.Prompt)
			t.RunTime = t.getDelta()
			t.commandLock.Unlock()
			continue
		} else if err == io.EOF {
			currentEnv = ""
			i = len(commands)
			t.commandLock.Lock()
			if t.Status == "" {
				t.Status = "crash"
			}
			t.RunTime = t.getDelta()
			t.commandLock.Unlock()
			continue
		} else if err != nil {
			return err
		}
		prompt := match.Groups[0]
		if prompt == KERNEL_PROMPT {
			currentEnv = "kernel"
		} else if prompt == SHELL_PROMPT {
			currentEnv = "shell"
		} else {
			return errors.New(fmt.Sprintf("test161: found invalid prompt: %s", prompt))
		}
	}
	return nil
}

func (t *Test) OutputJSON() (string, error) {
	outputBytes, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return "", err
	}
	return string(outputBytes), nil
}

func (t *Test) OutputString() string {
	var output string
	for _, conf := range strings.Split(t.ConfString, "\n") {
		conf = strings.TrimSpace(conf)
		output += fmt.Sprintf("conf: %s\n", conf)
	}
	for i, command := range t.Commands {
		for j, outputLine := range command.Output {
			if i == 0 || j != 0 {
				output += fmt.Sprintf("%.6f\t%s", outputLine.Delta, outputLine.Line)
			} else {
				output += fmt.Sprintf("%s", outputLine.Line)
			}
		}
	}
	output += t.Status
	if t.ShutdownMessage != "" {
		output += fmt.Sprintf(": %s", t.ShutdownMessage)
	}
	output += "\n"
	return output
}
