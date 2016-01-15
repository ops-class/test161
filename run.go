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

	sys161        *expect.Expect // Protected by L
	progressTime  float64        // Protected by L
	command       *Command       // Protected by L
	currentOutput OutputLine     // Protected by L

	// Fields used by etStats but shared with Run
	statCond    *sync.Cond // Used by the main loop to wait for stat reception
	statError   error      // Protected by statCond.L
	statActive  bool       // Protected by statCond.L
	statRecord  bool       // Protected by statCond.L
	statMonitor bool       // Protected by statCond.L

	// Output channels
	statChan chan Stat // Nonblocking write
}

type Command struct {
	Counter      uint         `json:"counter"`
	ID           uint         `json:"-"`
	Env          string       `json:"env"`
	Input        InputLine    `json:"input"`
	Output       []OutputLine `json:"output"`
	SummaryStats Stat         `json:"summarystats"`
	AllStats     []Stat       `json:"stats"`
	Retries      uint         `json:"retries"`
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

	// Serialize the current command state.
	t.L = &sync.Mutex{}

	// Coordinated with the getStat goroutine. I don't think that a channel
	// would work here.
	t.statCond = &sync.Cond{L: &sync.Mutex{}}

	// Initialize stat channel. Closed by getStats
	t.statChan = make(chan Stat)

	// Record stats during boot, but don't active the monitor.
	t.statRecord = true
	t.statMonitor = false

	// Wait for either kernel or user prompts.
	prompts := regexp.MustCompile(PROMPT_PATTERN)

	// Parse commands and counters:
	//
	// 		commandIndex: holds the index into the commands array and usually the
	// 		current command. -1 means boot and len(commands) means we are in the
	// 		shutdown sequence.
	//
	//    commandCounter: monotonically-increasing command counter. Not
	//    increased when commands are repeated due to output mismatches. Used
	//    for output indexing.
	//
	//		commandID: monotonically-increasing command counter that _does_ bump
	//		when we repeat commands. Shared wiht getStats for command
	//		identification.
	//
	// Note that a zero-length commands string is legitimate, causing boot and
	// immediate shutdown.
	commands := strings.Split(strings.TrimSpace(t.Content), "\n")
	commandIndex := BOOT
	commandCounter := BOOT
	commandID := BOOT

	// We increased the index during the last time through
	bumpedIndex := false
	// Retry count for failed commands
	retryCount := uint(0)

	// Check for empty commands before starting sys161.
	for _, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			return errors.New("test161: found empty command")
		}
	}

	// Boot environment is the kernel.
	currentEnv := "kernel"

	// Flag for final pass to grab exit output.
	finished := false

	// Main command loop. Note that sys161 is not started until the first time
	// through, and the loop completes in the middle to mop up output from
	// previous commands.
	for {
		var commandLine string
		var statMonitor bool

		// Rotate running command to the next command, saving any previous
		// output as needed.
		t.L.Lock()
		if t.command != nil {
			if t.currentOutput.WallTime != 0.0 {
				t.currentOutput.Line = t.currentOutput.Buffer.String()
				t.command.Output = append(t.command.Output, t.currentOutput)
			}

			// We work hard to make sure that the previous command actually got
			// executed by comparing the expect output with what we send it. Sadly,
			// this does happen, particularly during parallel testing. No idea why.
			previousCommand := strings.TrimSpace(t.command.Input.Line)
			var expectedCommand string

			rollback := false
			if previousCommand != "boot" && !finished {
				// Did we get any output?
				if len(t.command.Output) > 0 {
					expectedCommand = strings.TrimSpace(t.command.Output[0].Line)
				}
				if previousCommand != expectedCommand {
					rollback = true
				}
			}

			if !rollback {
				t.Commands = append(t.Commands, *t.command)
				retryCount = 0
			} else {
				retryCount += 1
				if retryCount >= t.Misc.CommandRetries {
					t.Status = "expect"
					t.ShutdownMessage = fmt.Sprintf("couldn't echo command after %v retries", t.Misc.CommandRetries)
					t.WallTime = t.getWallTime()
					t.L.Unlock()
					return
				}

				if bumpedIndex {
					commandIndex -= 1
				}
				commandCounter -= 1
			}
			t.currentOutput = OutputLine{}
		}

		// Grab the next command, bumping counters as appropriate.
		if finished {
			// Shutdowns exit here after we save the previous output.
			t.L.Unlock()
			return nil
		} else if commandIndex == BOOT {
			commandLine = "boot"
			commandIndex += 1
			// Don't set bumpedIndex here since we're in boot
		} else if commandIndex < len(commands) {
			commandLine = strings.TrimSpace(commands[commandIndex])

			// Handle testfile syntatic sugar by getting back and forth to the menu
			// or shell. Added commands don't bump the commandIndex. If the command
			// fails the environment should stay the same and we'll retry
			// automatically.
			if string(commandLine[0]) == "$" && currentEnv == "kernel" {
				commandLine = "s"
				statMonitor = true
				bumpedIndex = false
				// This command quickly enters userspace and should be marked as such
				currentEnv = "user"
			} else if string(commandLine[0]) != "$" && currentEnv == "user" {
				commandLine = "exit"
				statMonitor = false
				bumpedIndex = false
			} else {
				if string(commandLine[0]) == "$" {
					commandLine = strings.TrimSpace(commandLine[1:])
				}
				// Mark other commands that run in userspace, including "p" which
				// launches from the kernel menu.
				if currentEnv == "kernel" && commandLine == "s" || strings.HasPrefix(commandLine, "p ") {
					currentEnv = "user"
				}
				statMonitor = true
				commandIndex += 1
				bumpedIndex = true
			}
		} else {
			statMonitor = false
			// Shutdown cleanly if needed.
			if currentEnv == "kernel" {
				commandLine = "q"
			} else if currentEnv == "user" {
				commandLine = "exit"
			}
		}
		// Shutdown the monitor during shutdown
		if currentEnv == "kernel" && commandLine == "q" {
			statMonitor = false
		}

		// Bump counters
		commandCounter += 1
		commandID += 1

		t.command = &Command{
			Counter: uint(commandCounter),
			ID:      uint(commandID),
			Env:     currentEnv,
			Input:   InputLine{WallTime: t.getWallTime(), SimTime: t.SimTime, Line: commandLine},
			Retries: retryCount,
		}
		t.L.Unlock()

		if t.Status == "aborted" {
			// Start sys161 if needed and defer Close.
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
			err = t.sys161.Send(commandLine)
			if err != nil {
				return err
			}

			// Flip on stat monitoring (except during shutdown) and stat recording
			// always.
			t.statCond.L.Lock()
			t.statMonitor = statMonitor
			t.statRecord = true

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
		t.statRecord = false
		t.statMonitor = false
		err = t.statError
		t.statCond.L.Unlock()
		if err != nil {
			return err
		}

		// Handle timeouts, unexpected shutdowns, and other errors
		err = expectErr
		if err == expect.ErrTimeout {
			t.L.Lock()
			t.Status = "timeout"
			t.ShutdownMessage = fmt.Sprintf("no prompt for %v s", t.Misc.PromptTimeout)
			t.WallTime = t.getWallTime()
			t.L.Unlock()
			finished = true
			continue
		} else if err == io.EOF {
			t.L.Lock()
			// Triggered normally or not?
			if t.Status == "started" {
				if currentEnv == "kernel" && commandLine == "q" {
					t.Status = "shutdown"
				} else {
					t.Status = "crash"
				}
			}
			t.WallTime = t.getWallTime()
			t.L.Unlock()
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

		if commandLine == "boot" && prompt != KERNEL_PROMPT {
			// Handle incorrect boot prompt. We shouldn't boot into the shell!
			return errors.New(fmt.Sprintf("test161: incorrect prompt at boot: %s", prompt))
		} else if prompt == KERNEL_PROMPT {
			currentEnv = "kernel"
		} else if prompt == SHELL_PROMPT {
			currentEnv = "user"
		} else {
			return errors.New(fmt.Sprintf("test161: found invalid prompt: %s", prompt))
		}
	}
	return nil
}

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
func (t *Test) stop161() {
	t.sys161.Close()
}
