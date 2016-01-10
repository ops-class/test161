package test161

import (
	"bufio"
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
	"reflect"
	"regexp"
	"strconv"
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

var validStat = regexp.MustCompile(`^DATA\s+(?P<Kern>\d+)\s+(?P<User>\d+)\s+(?P<Idle>\d+)\s+(?P<Kinsns>\d+)\s+(?P<Uinsns>\d+)\s+(?P<IRQs>\d+)\s+(?P<Exns>\d+)\s+(?P<Disk>\d+)\s+(?P<Con>\d+)\s+(?P<Emu>\d+)\s+(?P<Net>\d+)`)

type Stat struct {
	initialized bool
	Start       TimeDelta `json:"start"`
	End         TimeDelta `json:"end"`
	Length      TimeDelta `json:"length"`
	Kern        uint32    `json:"kern"`
	User        uint32    `json:"user"`
	Idle        uint32    `json:"idle"`
	Kinsns      uint32    `json:"kinsns"`
	Uinsns      uint32    `json:"uinsns"`
	IRQs        uint32    `json:"irqs"`
	Exns        uint32    `json:"exns"`
	Disk        uint32    `json:"disk"`
	Con         uint32    `json:"con"`
	Emu         uint32    `json:"emu"`
	Net         uint32    `json:"net"`
}

func (i *Stat) Add(j Stat) {
	i.Kern += j.Kern
	i.User += j.User
	i.Idle += j.Idle
	i.Kinsns += j.Kinsns
	i.Uinsns += j.Uinsns
	i.IRQs += j.IRQs
	i.Exns += j.Exns
	i.Disk += j.Disk
	i.Con += j.Con
	i.Emu += j.Emu
	i.Net += j.Net
}
func (i *Stat) Sub(j Stat) {
	i.Kern -= j.Kern
	i.User -= j.User
	i.Idle -= j.Idle
	i.Kinsns -= j.Kinsns
	i.Uinsns -= j.Uinsns
	i.IRQs -= j.IRQs
	i.Exns -= j.Exns
	i.Disk -= j.Disk
	i.Con -= j.Con
	i.Emu -= j.Emu
	i.Net -= j.Net
}
func (i *Stat) Merge(j Stat) {
	if i.initialized == false {
		i.Start = j.Start
		i.initialized = true
	}
	i.End = j.End
	i.Length = TimeDelta(float64(i.End) - float64(i.Start))
	i.Add(j)
}

type TimeDelta float64

func (t TimeDelta) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%.6f", t)), nil
}

func (t *Test) stopStats() {
	t.statCond.L.Lock()
	t.statActive = false
	t.statCond.Signal()
	t.statCond.L.Unlock()
}

func (t *Test) getStats(statConn net.Conn) {
	defer t.stopStats()

	statReader := bufio.NewReader(statConn)

	var err error
	var line string

	start := t.getDelta()
	lastStat := Stat{}

	statCache := make([]Stat, 0, t.MonitorConf.Intervals)

	for {
		if err == nil {
			line, err = statReader.ReadString('\n')
		}
		end := t.getDelta()

		t.statCond.L.Lock()
		t.statActive = true
		if err != nil && err != io.EOF {
			t.statError = err
		}
		t.statCond.Signal()
		t.statCond.L.Unlock()

		if err != nil {
			return
		}

		statMatch := validStat.FindStringSubmatch(line)
		if statMatch == nil {
			continue
		}

		newStats := Stat{
			Start:  start,
			End:    end,
			Length: TimeDelta(float64(end) - float64(start)),
		}

		start = end
		s := reflect.ValueOf(&newStats).Elem()
		for i, name := range validStat.SubexpNames() {
			f := s.FieldByName(name)
			x, err := strconv.ParseUint(statMatch[i], 10, 32)
			if err != nil {
				continue
			}
			f.SetUint(x)
		}

		tempStat := newStats
		newStats.Sub(lastStat)
		lastStat = tempStat

		t.commandLock.Lock()
		if len(t.command.AllStats) == 0 {
			statCache = make([]Stat, 0, t.MonitorConf.Intervals)
		}
		t.command.AllStats = append(t.command.AllStats, newStats)
		t.command.SummaryStats.Merge(newStats)
		currentCommandID := t.command.ID
		currentEnv := t.command.Env
		t.commandLock.Unlock()

		if t.MonitorConf.Enabled != "true" {
			continue
		}
		if uint(len(statCache)) == t.MonitorConf.Intervals {
			statCache = statCache[1:]
		}
		statCache = append(statCache, newStats)
		if uint(len(statCache)) < t.MonitorConf.Intervals {
			continue
		}
		intervalStat := &Stat{}
		for _, stat := range statCache {
			intervalStat.Merge(stat)
		}

		monitorError := ""

		if currentEnv == "kernel" && intervalStat.User > 0 {
			monitorError = "non-zero user cycles during kernel operation"
		}

		if float64(intervalStat.Kern)/
			float64(intervalStat.Kern+intervalStat.User+intervalStat.Idle) < t.MonitorConf.Kernel.Min {
			monitorError = "insufficient kernel cycle (potential deadlock)"
		}
		if float64(intervalStat.Kern)/
			float64(intervalStat.Kern+intervalStat.User+intervalStat.Idle) > t.MonitorConf.Kernel.Max {
			monitorError = "too many kernel cycle (potential livelock)"
		}

		if currentEnv == "shell" && (float64(intervalStat.User)/
			float64(intervalStat.Kern+intervalStat.User+intervalStat.Idle) < t.MonitorConf.User.Min) {
			monitorError = "insufficient user cycles"
		}
		if currentEnv == "shell" && (float64(intervalStat.User)/
			float64(intervalStat.Kern+intervalStat.User+intervalStat.Idle) > t.MonitorConf.User.Max) {
			monitorError = "too many user cycles"
		}

		if monitorError != "" {
			t.commandLock.Lock()
			if currentCommandID == t.command.ID {
				t.Status = "monitor"
				t.ShutdownMessage = monitorError
				t.sys161.Killer()
			}
			t.commandLock.Unlock()
		}
	}
}

func (t *Test) getDelta() TimeDelta {
	return TimeDelta(float64(time.Now().UnixNano()-t.startTime) / float64(1000*1000*1000))
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
