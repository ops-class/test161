package test161

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ericaro/frontmatter"
	"github.com/jamesharr/expect"
	"github.com/termie/go-shutil"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"
	"unicode"
	// "gopkg.in/yaml.v2"
)

const KERNEL_PROMPT = `OS/161 kernel [? for menu]: `
const SHELL_PROMPT = `OS/161$ `

type DiskConfig struct {
	RPM     uint   `yaml:"rpm"`
	Sectors string `yaml:"sectors"`
	Bytes   string
	NoDoom  string `yaml:"nodoom"`
	File    string
}

var validRandom = regexp.MustCompile(`(autoseed|seed=\d+)`)

type Config struct {
	CPUs   uint       `yaml:"cpus"`
	RAM    string     `yaml:"ram"`
	Random string     `yaml:"random"`
	Disk1  DiskConfig `yaml:"disk1"`
	Disk2  DiskConfig `yaml:"disk2"`
}

type TimeDelta float64

func (t TimeDelta) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%.6f", t)), nil
}

var validStat = regexp.MustCompile(`^DATA\s+(?P<Kern>\d+)\s+(?P<User>\d+)\s+(?P<Idle>\d+)\s+(?P<Kinsns>\d+)\s+(?P<Uinsns>\d+)\s+(?P<IRQs>\d+)\s+(?P<Exns>\d+)\s+(?P<Disk>\d+)\s+(?P<Con>\d+)\s+(?P<Emu>\d+)\s+(?P<Net>\d+)`)

type Stat struct {
	Delta  TimeDelta `json:"delta"`
	Kern   uint32    `json:"kern"`
	User   uint32    `json:"user"`
	Idle   uint32    `json:"idle"`
	Kinsns uint32    `json:"kinsns"`
	Uinsns uint32    `json:"uinsns"`
	IRQs   uint32    `json:"irqs"`
	Exns   uint32    `json:"exns"`
	Disk   uint32    `json:"disk"`
	Con    uint32    `json:"con"`
	Emu    uint32    `json:"emu"`
	Net    uint32    `json:"net"`
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

type Command struct {
	Env          string       `json:"env"`
	Input        InputLine    `json:"input"`
	Output       []OutputLine `json:"output"`
	SummaryStats Stat         `json:"-"`
	AllStats     []Stat       `json:"stats"`
}

type CommandOutput struct {
	JSON string
}

type Test struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
	Depends     []string `yaml:"depends"`
	Timeout     uint     `yaml:"timeout"`
	Content     string   `fm:"content" yaml:"-"`

	Conf     Config `yaml:"-"`
	OrigConf Config `yaml:"conf"`

	sys161    *expect.Expect
	startTime int64

	statCond  *sync.Cond
	statError error

	commandLock   *sync.Mutex
	command       *Command
	currentOutput OutputLine

	Output struct {
		Status   string    `json:"status"`
		RunTime  TimeDelta `json:"runtime"`
		Commands []Command `json:"commands"`
	}
}

func parseAndSetDefault(in string, backup string, unit int) (string, error) {
	if in == "" {
		in = backup
	}
	if unit == 0 {
		unit = 1
	}
	if unicode.IsDigit(rune(in[len(in)-1])) {
		number, err := strconv.Atoi(in)
		if err != nil {
			return "", err
		}
		return strconv.Itoa(number / unit), nil
	} else {
		number, err := strconv.Atoi(in[0 : len(in)-1])
		if err != nil {
			return "", err
		}
		multiplier := strings.ToUpper(string(in[len(in)-1]))
		if multiplier == "K" {
			return strconv.Itoa(1024 * number / unit), nil
		} else if multiplier == "M" {
			return strconv.Itoa(1024 * 1024 * number / unit), nil
		} else {
			return "", errors.New("test161: could not convert formatted string to integer")
		}
	}
}

// LoadTest parses the test file and sets configuration defaults.
func LoadTest(filename string) (*Test, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	test := new(Test)
	err = frontmatter.Unmarshal(data, test)
	if err != nil {
		return nil, err
	}

	test.Conf.CPUs = test.OrigConf.CPUs
	if test.Conf.CPUs == 0 {
		test.Conf.CPUs = 4
	}
	test.Conf.RAM, err = parseAndSetDefault(test.OrigConf.RAM, "1M", 1)
	if err != nil {
		return nil, err
	}
	ramInt, _ := strconv.Atoi(test.Conf.RAM)
	ramInt *= 2

	// 09 Jan 2015 : GWA : sys161 currently won't boot with a disk smaller than
	// 8000 sectors. Not sure why.

	if ramInt < 8000*512 {
		ramInt = 8000 * 512
	}
	ramString := strconv.Itoa(ramInt)

	test.Conf.Disk1.RPM = test.OrigConf.Disk1.RPM
	if test.Conf.Disk1.RPM == 0 {
		test.Conf.Disk1.RPM = 7200
	}
	test.Conf.Disk1.Sectors, err =
		parseAndSetDefault(test.OrigConf.Disk1.Sectors, ramString, 512)
	if err != nil {
		return nil, err
	}
	test.Conf.Disk1.Bytes, err =
		parseAndSetDefault(test.OrigConf.Disk1.Sectors, ramString, 1)
	if err != nil {
		return nil, err
	}
	test.Conf.Disk1.NoDoom = test.OrigConf.Disk1.NoDoom
	if test.Conf.Disk1.NoDoom == "" {
		test.Conf.Disk1.NoDoom = "true"
	}
	switch test.Conf.Disk1.NoDoom {
	case "true", "false":
		break
	default:
		return nil, errors.New("test161: nodoom must be 'true' or 'false' if set.")
	}
	test.Conf.Disk1.File = "LDH0.img"

	if test.OrigConf.Disk2.RPM != 0 ||
		test.OrigConf.Disk2.Sectors != "" ||
		test.OrigConf.Disk2.NoDoom != "" {

		test.Conf.Disk2.RPM = test.OrigConf.Disk2.RPM
		if test.Conf.Disk2.RPM == 0 {
			test.Conf.Disk2.RPM = 7200
		}
		test.Conf.Disk2.Sectors, err = parseAndSetDefault(test.OrigConf.Disk2.Sectors, "5M", 512)
		if err != nil {
			return nil, err
		}
		test.Conf.Disk2.Bytes, err = parseAndSetDefault(test.OrigConf.Disk2.Sectors, "5M", 1)
		if err != nil {
			return nil, err
		}
		test.Conf.Disk2.NoDoom = test.OrigConf.Disk2.NoDoom
		if test.Conf.Disk2.NoDoom == "" {
			test.Conf.Disk2.NoDoom = "false"
		}
		switch test.Conf.Disk2.NoDoom {
		case "true", "false":
			break
		default:
			return nil, errors.New("test161: nodoom must be 'true' or 'false' if set.")
		}
		test.Conf.Disk2.File = "LDH1.img"
	}

	test.Conf.Random = test.OrigConf.Random
	if test.Conf.Random == "" {
		test.Conf.Random = "seed=" + strconv.Itoa(int(rand.Int31()>>16))
	}
	if !validRandom.MatchString(test.Conf.Random) {
		return nil, errors.New("test161: random must be 'autoseed' or 'seed=N' if set.")
	}

	test.Timeout = test.Timeout
	if test.Timeout == 0 {
		test.Timeout = 10
	}
	return test, err
}

// PrintConf formats the test configuration for use by sys161 via the sys161.conf file.
func (t *Test) PrintConf() (string, error) {
	const base = `0	serial
1	emufs{{if .Disk1.Sectors}}
2	disk	rpm={{.Disk1.RPM}}	file={{.Disk1.File}} {{if eq .Disk1.NoDoom "true"}}nodoom{{end}}{{end}}{{if .Disk2.Sectors}}
3	disk	rpm={{.Disk2.RPM}}	file={{.Disk2.File}} {{if eq .Disk2.NoDoom "true"}}nodoom{{end}}{{end}}
28	random {{.Random}}
29	timer
30	trace
31	mainboard ramsize={{.RAM}} cpus={{.CPUs}}`

	conf, err := template.New("conf").Parse(base)
	if err != nil {
		return "", err
	}
	buffer := new(bytes.Buffer)
	err = conf.Execute(buffer, t.Conf)
	if err != nil {
		return "", err
	}
	return buffer.String(), nil
}

func (t *Test) getStats(statConn net.Conn) {
	statReader := bufio.NewReader(statConn)
	var err error
	var line string
	for {
		if err == nil {
			line, err = statReader.ReadString('\n')
		}
		t.statCond.L.Lock()
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
			Delta: t.getDelta(),
		}
		s := reflect.ValueOf(&newStats).Elem()
		for i, name := range validStat.SubexpNames() {
			f := s.FieldByName(name)
			x, err := strconv.ParseUint(statMatch[i], 10, 32)
			if err != nil {
				continue
			}
			f.SetUint(x)
		}
		t.commandLock.Lock()
		t.command.AllStats = append(t.command.AllStats, newStats)
		t.commandLock.Unlock()
	}
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
	conf, err := t.PrintConf()
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(confTarget, []byte(conf), 0440)
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
	t.sys161.SetTimeout(time.Duration(t.Timeout) * time.Second)

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
		if currentEnv != "" {
			t.statCond.L.Lock()
			t.statCond.Wait()
			statError = t.statError
			t.statCond.L.Unlock()
		}
		if statError != nil {
			return statError
		}
		t.commandLock.Lock()
		if t.currentOutput.Delta != 0.0 {
			t.currentOutput.Line = t.currentOutput.Buffer.String()
			t.command.Output = append(t.command.Output, t.currentOutput)
		}
		t.currentOutput = OutputLine{}
		t.Output.Commands = append(t.Output.Commands, *t.command)
		if command != "" {
			t.command = &Command{
				Env:   currentEnv,
				Input: InputLine{Delta: t.getDelta(), Line: command},
			}
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
			t.sys161.ExpectEOF()
			t.Output.Status = "shutdown"
			t.Output.RunTime = t.getDelta()
			continue
		}
		match, err := t.sys161.ExpectRegexp(prompts)
		if err == expect.ErrTimeout {
			currentEnv = ""
			i = len(commands)
			t.Output.Status = "timeout"
			t.Output.RunTime = t.getDelta()
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

func (t *Test) Recv(time time.Time, received []byte) {
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
	outputBytes, err := json.MarshalIndent(t.Output, "", "  ")
	if err != nil {
		return "", err
	}
	return string(outputBytes), nil
}

func (t *Test) OutputString() string {
	var output string
	for i, command := range t.Output.Commands {
		for j, outputLine := range command.Output {
			if i == 0 || j != 0 {
				output += fmt.Sprintf("%.6f\t%s", outputLine.Delta, outputLine.Line)
			} else {
				output += fmt.Sprintf("%s", outputLine.Line)
			}
		}
	}
	return output
}
