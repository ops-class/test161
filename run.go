package test161

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gchallen/expect"
	"github.com/kr/pty"
	"github.com/termie/go-shutil"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const KERNEL_PROMPT = `OS/161 kernel [? for menu]: `
const SHELL_PROMPT = `OS/161$ `

var copyLock = &sync.Mutex{}

type Command struct {
	ID           uint         `json:"-"`
	Env          string       `json:"env"`
	Input        InputLine    `json:"input"`
	Output       []OutputLine `json:"output"`
	SummaryStats Stat         `json:"summarystats"`
	AllStats     []Stat       `json:"-"`
}

type InputLine struct {
	WallTime TimeDelta `json:"walltime"`
	SimTime  TimeDelta `json:"simtime"`
	Line     string    `json:"line"`
}

type OutputLine struct {
	WallTime TimeDelta    `json:"walltime"`
	SimTime  TimeDelta    `json:"simtime"`
	Buffer   bytes.Buffer `json:"-"`
	Line     string       `json:"line"`
}

type TimeDelta float64

func (t TimeDelta) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%.6f", t)), nil
}

func (t *Test) getDelta() TimeDelta {
	return TimeDelta(float64(time.Now().UnixNano()-t.startTime) / float64(1000*1000*1000))
}

func (t *Test) Run(root string, tempRoot string) (err error) {

	// Exit status for configuration and initialization failures.
	t.Status = "aborted"

	if root == "" {
		return errors.New("test161: run requires a root directory")
	}

	// Create temp directory.
	tempRoot, err = ioutil.TempDir(tempRoot, "test161")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempRoot)
	t.tempDir = path.Join(tempRoot, "root")

	// Copy root.
	err = shutil.CopyTree(root, t.tempDir, nil)
	if err != nil {
		return err
	}

	// Make sure we have a kernel.
	kernelTarget := path.Join(t.tempDir, "kernel")
	_, err = os.Stat(kernelTarget)
	if err != nil {
		return err
	}

	// Generate an alternate configuration to prevent collisions.
	confTarget := path.Join(t.tempDir, "test161.conf")
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

	// Create disks.
	if t.Conf.Disk1.Sectors != "" {
		create := exec.Command("disk161", "create", t.Conf.Disk1.File, fmt.Sprintf("%ss", t.Conf.Disk1.Sectors))
		create.Dir = t.tempDir
		err = create.Run()
		if err != nil {
			return err
		}
	}
	if t.Conf.Disk2.Sectors != "" {
		create := exec.Command("disk161", "create", t.Conf.Disk2.File, fmt.Sprintf("%ss", t.Conf.Disk2.Sectors))
		create.Dir = t.tempDir
		err = create.Run()
		if err != nil {
			return err
		}
	}

	// Serialize the current command state.
	t.commandLock = &sync.Mutex{}

	// Coordinated with the getStat goroutine. I don't think that a channel
	// would work here.
	t.statCond = &sync.Cond{L: &sync.Mutex{}}

	// Initialize stat channel.
	t.statChan = make(chan Stat)

	// Record stats during boot, but don't active the monitor.
	t.recordStats = true
	t.monitorStats = false

	// Wait for either kernel or user prompts.
	prompts := regexp.MustCompile(fmt.Sprintf("(%s|%s)", regexp.QuoteMeta(KERNEL_PROMPT), regexp.QuoteMeta(SHELL_PROMPT)))

	// Parse commands and counters. i holds the current command index. -1 means
	// boot and > len(commands) means shutdown sequence. j holds a
	// monotonically-increasing command counter.
	//
	// Note that a zero-length commands string is legitimate, causing boot and
	// immediate shutdown.
	commands := strings.Split(strings.TrimSpace(t.Content), "\n")
	i := -1
	j := -1

	// Check for empty commands before starting sys161.
	for _, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			return errors.New("test161: found empty command")
		}
	}

	// Boot environment is blank.
	currentEnv := ""

	// Flag for final pass to grab exit output.
	finished := false

	// Main command loop. Note that sys161 is not started until the first time
	// through.
	for {
		// Grab the next command, bumping the counter if necessary.
		var command string
		var monitorStats bool

		if finished {
		} else if i == -1 {
			command = "boot"
			i += 1
		} else if i < len(commands) {
			command = strings.TrimSpace(commands[i])

			// Handle testfile syntatic sugar by getting back and forth to the menu.
			// Added commands don't bump the command counter.
			if string(command[0]) == "$" && currentEnv == "kernel" {
				command = "s"
				monitorStats = true
			} else if string(command[0]) != "$" && currentEnv == "shell" {
				command = "exit"
				monitorStats = false
			} else {
				if string(command[0]) == "$" {
					command = command[1:]
				}
				monitorStats = true
				i += 1
			}
		} else {
			monitorStats = false
			// Shutdown cleanly if needed.
			if currentEnv == "kernel" {
				command = "q"
			} else if currentEnv == "shell" {
				command = "exit"
			}
		}
		j += 1

		// Rotate running command to the next command, saving any previous
		// output as needed.
		t.commandLock.Lock()
		if t.currentOutput.WallTime != 0.0 {
			t.currentOutput.Line = t.currentOutput.Buffer.String()
			t.command.Output = append(t.command.Output, t.currentOutput)
		}
		if t.command != nil {
			t.Commands = append(t.Commands, *t.command)
		}
		t.currentOutput = OutputLine{}
		if !finished {
			t.command = &Command{
				ID:    uint(j),
				Env:   currentEnv,
				Input: InputLine{WallTime: t.getDelta(), SimTime: t.SimTime, Line: command},
			}
		}
		t.commandLock.Unlock()

		// Clean or unclean shutdowns exit here after we save the previous output.
		if finished {
			return nil
		}

		if t.Status == "aborted" {
			// Start sys161 if needed, with deferred close.
			err = t.start161()
			if err != nil {
				return err
			}
			defer t.stop161()
		} else {
			// Send the command. To start as cleanly as possible, we transmit
			// everything but the newline, wait for a stat signal, and then run the
			// command. With the exception of boot stat collection is disabled at this
			// point due to code below.
			err = t.sys161.Send(command)
			if err != nil {
				return err
			}

			// Flip on stat monitoring (except during shutdown) and stat recording
			// always.
			t.statCond.L.Lock()
			t.monitorStats = monitorStats
			t.recordStats = true

			// Wait for stat signal (and check for stat errors)...
			if t.statActive {
				t.statCond.Wait()
			}
			err = t.statError
			t.statCond.L.Unlock()
			if err != nil {
				return err
			}

			// Now go!
			t.sys161.Send("\n")
		}
		match, expectErr := t.sys161.ExpectRegexp(prompts)

		// Disable stat recording and monitoring (and check for stat errors).
		t.statCond.L.Lock()
		t.recordStats = false
		t.monitorStats = false
		err = t.statError
		t.statCond.L.Unlock()
		if err != nil {
			return err
		}

		// Handle timeouts, unexpected shutdowns, and other errors
		err = expectErr
		if err == expect.ErrTimeout {
			t.commandLock.Lock()
			t.Status = "timeout"
			t.ShutdownMessage = fmt.Sprintf("no prompt for %d s", t.MonitorConf.Timeouts.Prompt)
			t.WallTime = t.getDelta()
			t.commandLock.Unlock()
			finished = true
			continue
		} else if err == io.EOF {
			t.commandLock.Lock()
			// Triggered normally or not?
			if t.Status == "started" {
				if currentEnv == "kernel" && command == "q" {
					t.Status = "shutdown"
				} else {
					t.Status = "crash"
				}
			}
			t.WallTime = t.getDelta()
			t.commandLock.Unlock()
			finished = true
			continue
		} else if err != nil {
			return err
		}

		// Parse the prompt to set the environment
		if len(match.Groups) != 2 {
			return errors.New("test161: prompt didn't match")
		}
		prompt := match.Groups[0]

		if currentEnv == "" && prompt != KERNEL_PROMPT {
			// Handle expected boot prompt. We shouldn't boot into the shell!
			return errors.New("test161: incorrect prompt at boot")
		} else if prompt == KERNEL_PROMPT {
			currentEnv = "kernel"
		} else if prompt == SHELL_PROMPT {
			currentEnv = "shell"
		} else {
			return errors.New(fmt.Sprintf("test161: found invalid prompt: %s", prompt))
		}
	}
	return nil
}

func (t *Test) start161() error {
	// Fire up sys161. Note that the getStats goroutine starts now (started by
	// Rcev) as do Recv events, so we start synchronizing shared state now.
	run := exec.Command("sys161", "-X", "-c", "test161.conf", "-S", strconv.Itoa(int(t.MonitorConf.Resolution)), "kernel")
	run.Dir = t.tempDir
	pty, err := pty.Start(run)
	if err != nil {
		return err
	}
	killer := func() {
		run.Process.Kill()
	}
	t.sys161 = expect.Create(pty, killer)
	t.startTime = time.Now().UnixNano()
	t.sys161.SetLogger(t)
	t.sys161.SetTimeout(time.Duration(t.MonitorConf.Timeouts.Prompt) * time.Second)
	t.commandLock.Lock()
	if t.Status == "aborted" {
		t.Status = "started"
	}
	t.commandLock.Unlock()
	return nil
}

func (t *Test) stop161() {
	t.sys161.Close()
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
				output += fmt.Sprintf("%.6f\t%s", outputLine.SimTime, outputLine.Line)
			} else {
				output += fmt.Sprintf("%s", outputLine.Line)
			}
		}
	}
	if string(output[len(output)-1]) != "\n" {
		output += "\n"
	}
	output += fmt.Sprintf("%.6f\t%s", t.SimTime, t.Status)
	if t.ShutdownMessage != "" {
		output += fmt.Sprintf(": %s", t.ShutdownMessage)
	}
	output += "\n"
	return output
}
