/*
Package test161 implements a library for testing OS/161 kernels. We use expect
to drive the sys161 system simulator and collect useful output using the stat
socket.
*/
package test161

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/kr/pty"
	"github.com/ops-class/test161/expect"
	"github.com/termie/go-shutil"
	"io"
	"io/ioutil"
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
const PROMPT_PATTERN = `(OS/161 kernel \[\? for menu\]\:|OS/161\$)\s$`
const BOOT = -1

type Test struct {

	// Input

	// Metadata
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Tags        []string `yaml:"tags" json:"tags"`
	Depends     []string `yaml:"depends" json:"depends"`

	// Configuration chunks
	Sys161  Sys161Conf  `yaml:"sys161" json:"sys161"`
	Stat    StatConf    `yaml:"stat" json:"stat"`
	Monitor MonitorConf `yaml:"monitor" json:"monitor"`
	Misc    MiscConf    `yaml:"misc" json:"misc"`

	// Actual test commands to run
	Content string `fm:"content" yaml:"-" json:"-"`

	// Big lock that protects most fields shared between Run and getStats
	L *sync.Mutex `json:"-"`

	// Output

	ConfString      string         `json:"confstring"`      // Only set during once
	Status          string         `json:"status"`          // Protected by L
	ShutdownMessage string         `json:"shutdownmessage"` // Protected by L
	WallTime        TimeFixedPoint `json:"walltime"`        // Protected by L
	SimTime         TimeFixedPoint `json:"simtime"`         // Protected by L
	Commands        []Command      `json:"commands"`        // Protected by L

	// Unproctected Private fields
	tempDir     string // Only set once
	startTime   int64  // Only set once
	statStarted bool   // Only changed once

	sys161         *expect.Expect // Protected by L
	progressTime   float64        // Protected by L
	currentCommand *Command       // Protected by L
	currentOutput  OutputLine     // Protected by L

	// Fields used by getStats but shared with Run
	statCond    *sync.Cond // Used by the main loop to wait for stat reception
	statError   error      // Protected by statCond.L
	statRecord  bool       // Protected by statCond.L
	statMonitor bool       // Protected by statCond.L

	// Output channels
	statChan chan Stat // Nonblocking write
}

type Command struct {
	Type         string       `json:"type"`
	Input        InputLine    `json:"input"`
	Output       []OutputLine `json:"output"`
	SummaryStats Stat         `json:"summarystats"`
	AllStats     []Stat       `json:"stats"`
	Monitored    bool         `json:"monitored"`
}

type InputLine struct {
	WallTime TimeFixedPoint `json:"walltime"`
	SimTime  TimeFixedPoint `json:"simtime"`
	Line     string         `json:"line"`
}

type OutputLine struct {
	WallTime TimeFixedPoint `json:"walltime"`
	SimTime  TimeFixedPoint `json:"simtime"`
	Buffer   bytes.Buffer   `json:"-"`
	Line     string         `json:"line"`
}

type TimeFixedPoint float64

// MarshalJSON prints our TimeFixedPoint type as a fixed point float for JSON.
func (t TimeFixedPoint) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%.6f", t)), nil
}

// getTimeFixedPoint returns the current wall clock time as a TimeFixedPoint
func (t *Test) getWallTime() TimeFixedPoint {
	return TimeFixedPoint(float64(time.Now().UnixNano()-t.startTime) / float64(1000*1000*1000))
}

// Run a test161 test.
func (t *Test) Run(root string) (err error) {

	// Exit status for configuration and initialization failures.
	t.Status = "aborted"

	// Merge in test161 defaults for any missing configuration values
	err = t.MergeConf(CONF_DEFAULTS)
	if err != nil {
		return err
	}

	// Create temp directory.
	tempRoot, err := ioutil.TempDir(t.Misc.TempDir, "test161")
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
	if t.Sys161.Disk1.Enabled == "true" {
		create := exec.Command("disk161", "create", "LHD0.img", t.Sys161.Disk1.Bytes)
		create.Dir = t.tempDir
		err = create.Run()
		if err != nil {
			return err
		}
	}
	if t.Sys161.Disk2.Enabled == "true" {
		create := exec.Command("disk161", "create", "LHD1.img", t.Sys161.Disk2.Bytes)
		create.Dir = t.tempDir
		err = create.Run()
		if err != nil {
			return err
		}
	}

	// Check for empty commands and expand syntatic sugar before getting
	// started. Doing this first makes the main loop and retry logic simpler.
	err = t.initCommands()
	if err != nil {
		return err
	}

	// Serialize the current command state.
	t.L = &sync.Mutex{}

	// Coordinated with the getStat goroutine. I don't think that a channel
	// would work here.
	t.statCond = &sync.Cond{L: &sync.Mutex{}}

	// Initialize stat channel. Closed by getStats
	t.statChan = make(chan Stat)

	// Record stats during boot, but don't activate the monitor.
	t.statRecord = true
	t.statMonitor = false

	// Wait for either kernel or user prompts.
	prompts := regexp.MustCompile(PROMPT_PATTERN)

	// Set up the current command to point at boot
	t.currentCommand = &t.Commands[0]

	// Start sys161 and defer close.
	err = t.start161()
	if err != nil {
		return err
	}
	defer t.stop161()

	for commandCounter, _ := range t.Commands {
		if commandCounter != 0 {
			err = t.sendCommand(t.currentCommand.InputLine.Line + "\n")
			if err != nil {
				t.finish("expect", "couldn't send a command")
				return nil
			}
			err = t.enableStats()
			if err != nil {
				t.finish("stats", "stats error")
				return nil
			}
		}

		if commandCounter == len(t.Commands)-1 {
			t.sys161.ExpectEOF()
			t.finish("shutdown", "")
			return nil
		} else {
			prompt, expectErr := t.sys161.ExpectRegexp(prompts)
			err = t.disableStats()
			if err != nil {
				t.finish("stats", "stats error")
				return nil
			}
		}

		// Handle timeouts, unexpected shutdowns, and other errors
		if expectErr == expect.ErrTimeout {
			t.finish("timeout", fmt.Sprintf("no prompt for %v s", t.Misc.PromptTimeout))
			return nil
		} else if err == io.EOF {
			t.finish("crash", "")
			return nil
		} else if err != nil {
			t.finish("expect", "")
			return nil
		}

		// Rotate running command to the next command, saving any previous
		// output as needed.
		t.L.Lock()
		if t.currentOutput.WallTime != 0.0 {
			t.currentOutput.Line = t.currentOutput.Buffer.String()
			t.currentCommand.Output = append(t.currentCommand.Output, t.currentOutput)
		}
		t.currentOutput = OutputLine{}
		t.currentCommand = &t.Commands[commandCounter+1]
		t.L.Unlock()

		// Check the prompt against the expected environment
		if len(match.Groups) != 2 {
			t.finish("expect", "prompt didn't match")
			return nil
		}
		prompt := match.Groups[0]

		if prompt == KERNEL_PROMPT {
			if t.currentCommand.Type != "kernel" {
				t.finish("expect", "prompt doesn't match kernel environment")
			}
		} else if prompt == USER_PROMPT {
			if t.currentCommand.Type != "user" {
				t.finish("expect", "prompt doesn't match user environment")
			}
		} else {
			t.finish("expect", "found invalid prompt: %s", prompt)
			return nil
		}
	}
	return nil
}

// sendCommand sends a command persistently. All the retry logic to deal with
// dropped characters is now here.
func (t *Test) sendCommand(commandLine string, retryLimit int) error {
	t.sys161.SetTimeout(time.Second)
	defer t.sys161.SetTimeout(time.Duration(t.Misc.PromptTimeout) * time.Second)

	for _, character := range commandLine {
		for retryCount := 0; retryCount < retryLimit; retryCount++ {
			err := t.sys161.Send(character)
			if err {
				return err
			}
			_, err := t.sys161.ExpectRegexp(regexp.QuoteMeta(character))
			if err == nil {
				break
			} else if err == expect.ErrTimeout {
				continue
			} else {
				return err
			}
		}
		if retryCount == retryLimit {
			return errors.New("test161: timeout sending command")
		}
	}

	return nil
}

// Lifecycle functions

// start161 is a private helper function to start the sys161 expect process.
// This makes the main loop a bit cleaner.
func (t *Test) start161() error {
	run := exec.Command("sys161", "-X", "-c", "test161.conf", "kernel")
	run.Dir = t.tempDir
	pty, err := pty.Start(run)
	if err != nil {
		return err
	}
	killer := func() {
		run.Process.Kill()
	}
	// Set timeout at create. Otherwise expect uses a ridiculous value and we
	// can hang with early failures.
	t.sys161 = expect.Create(pty, killer, t, time.Duration(t.Misc.PromptTimeout)*time.Second)
	t.startTime = time.Now().UnixNano()
	t.sys161.SetTimeout(time.Duration(t.Misc.PromptTimeout) * time.Second)
	t.L.Lock()
	if t.Status == "aborted" {
		t.Status = "started"
	}
	t.L.Unlock()
	return nil
}

// start161 is a private helper function to stop the sys161 expect process.
// Defered to the end of Run.
func (t *Test) finish(status string, shutdownMessage string) {
	t.L.Lock()
	if t.Status == "" {
		t.Status = status
		t.ShutdownMessage = shutdownMessage
	}
	t.WallTime = t.getWallTime()
	t.L.Unlock()
}

// start161 is a private helper function to stop the sys161 expect process.
// Defered to the end of Run.
func (t *Test) stop161() {
	t.sys161.Close()
}
