package test161

import (
	"bytes"
	"errors"
	"github.com/ericaro/frontmatter"
	"github.com/gchallen/expect"
	"io/ioutil"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"unicode"
)

type Test struct {
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Tags        []string `yaml:"tags" json:"tags"`
	Depends     []string `yaml:"depends" json:"depends"`
	Content     string   `fm:"content" yaml:"-"`

	Conf     Conf `yaml:"-" json:"conf"`
	OrigConf Conf `yaml:"conf" json:"origconf"`

	MonitorConf MonitorConf `yaml:"monitor" json:"monitor"`

	sys161    *expect.Expect
	tempDir   string
	startTime int64

	statChan chan Stat

	statCond    *sync.Cond
	statStarted bool  // unprotected; only used once
	statError   error // protected by statCond.L
	statActive  bool  // protected by statCond.L
	statRecord  bool  // protected by statCond.L
	statMonitor bool  // protected by statCond.L

	progressTime float64

	commandLock   *sync.Mutex
	command       *Command
	currentOutput OutputLine

	ConfString      string         `json:"confstring"`
	Status          string         `json:"status"`
	ShutdownMessage string         `json:"shutdownmessage"`
	WallTime        TimeFixedPoint `json:"walltime"`
	SimTime         TimeFixedPoint `json:"simtime"`
	Commands        []Command      `json:"commands"`
}

var validRandom = regexp.MustCompile(`(autoseed|seed=\d+)`)

type Conf struct {
	CPUs   uint     `yaml:"cpus" json:"cpus"`
	RAM    string   `yaml:"ram" json:"ram"`
	Random string   `yaml:"random" json:"random"`
	Disk1  DiskConf `yaml:"disk1" json:"disk1"`
	Disk2  DiskConf `yaml:"disk2" json:"disk2"`
}

type DiskConf struct {
	RPM     uint   `yaml:"rpm" json:"rpm"`
	Sectors string `yaml:"sectors" json:"sectors"`
	NoDoom  string `yaml:"nodoom" json:"nodoom"`
	File    string
}

type MonitorConf struct {
	AllStats   string   `yaml:"allstats" json:"allstats"`
	Enabled    string   `yaml:"enabled" json:"enabled"`
	Resolution uint     `yaml:"resolution" json:"resolution"`
	Window     float32  `yaml:"window" json:"window"`
	Kernel     Limits   `yaml:"kernel" json:"kernel"`
	User       Limits   `yaml:"user" json:"user"`
	Timeouts   Timeouts `yaml:"timeouts" json:"timeouts"`
}

type Limits struct {
	Min float64 `yaml:"min" json:"min"`
	Max float64 `yaml:"max" json:"max"`
}

type Timeouts struct {
	Prompt   uint `yaml:"prompt" json:"prompt"`
	Progress uint `yaml:"progress" json:"progress"`
}

// parseAndSetDefault converts prefixes appropriately and uses defaults when
// no value is supplied. Prefixes should be available soon in sys161 so this
// can get simpler.
func parseAndSetDefault(in string, backup string) (string, error) {
	if in == "" {
		in = backup
	}
	if unicode.IsDigit(rune(in[len(in)-1])) {
		return in, nil
	} else {
		number, err := strconv.Atoi(in[0 : len(in)-1])
		if err != nil {
			return "", err
		}
		multiplier := strings.ToUpper(string(in[len(in)-1]))
		if multiplier == "K" {
			return strconv.Itoa(1024 * number), nil
		} else if multiplier == "M" {
			return strconv.Itoa(1024 * 1024 * number), nil
		} else {
			return "", errors.New("test161: could not convert formatted string to integer")
		}
	}
}

// TestFromFile parses the test file and sets configuration defaults.
func TestFromFile(filename string) (*Test, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return TestFromString(string(data))
}

// TestFromFile parses the test string and sets configuration defaults.
func TestFromString(data string) (*Test, error) {
	test := new(Test)
	err := frontmatter.Unmarshal([]byte(data), test)
	if err != nil {
		return nil, err
	}

	test.Conf.CPUs = test.OrigConf.CPUs
	if test.Conf.CPUs == 0 {
		test.Conf.CPUs = 8
	}
	test.Conf.RAM, err = parseAndSetDefault(test.OrigConf.RAM, "1M")
	if err != nil {
		return nil, err
	}
	ramInt, _ := strconv.Atoi(test.Conf.RAM)
	ramInt = ramInt * 2 / 512

	// sys161 currently won't boot with a disk smaller than 8000 sectors. Not
	// sure why.

	if ramInt < 8000 {
		ramInt = 8000
	}
	ramString := strconv.Itoa(ramInt)

	test.Conf.Disk1.RPM = test.OrigConf.Disk1.RPM
	if test.Conf.Disk1.RPM == 0 {
		test.Conf.Disk1.RPM = 7200
	}
	test.Conf.Disk1.Sectors, err =
		parseAndSetDefault(test.OrigConf.Disk1.Sectors, ramString)
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
	test.Conf.Disk1.File = "LHD0.img"

	if test.OrigConf.Disk2.RPM != 0 ||
		test.OrigConf.Disk2.Sectors != "" ||
		test.OrigConf.Disk2.NoDoom != "" {

		test.Conf.Disk2.RPM = test.OrigConf.Disk2.RPM
		if test.Conf.Disk2.RPM == 0 {
			test.Conf.Disk2.RPM = 7200
		}
		test.Conf.Disk2.Sectors, err = parseAndSetDefault(test.OrigConf.Disk2.Sectors, ramString)
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
		test.Conf.Disk2.File = "LHD1.img"
	}

	test.Conf.Random = test.OrigConf.Random
	if test.Conf.Random == "" {
		test.Conf.Random = "seed=" + strconv.Itoa(int(rand.Int31()>>16))
	}
	if !validRandom.MatchString(test.Conf.Random) {
		return nil, errors.New("test161: random must be 'autoseed' or 'seed=N' if set.")
	}

	if test.MonitorConf.AllStats == "" {
		test.MonitorConf.AllStats = "false"
	}

	switch test.MonitorConf.AllStats {
	case "true", "false":
		break
	default:
		return nil, errors.New("test161: allstats must be 'true' or 'false' if set.")
	}

	if test.MonitorConf.Enabled == "" {
		test.MonitorConf.Enabled = "true"
	}
	if test.MonitorConf.Timeouts.Prompt == 0 {
		test.MonitorConf.Timeouts.Prompt = 5 * 60
	}
	if test.MonitorConf.Timeouts.Progress == 0 {
		test.MonitorConf.Timeouts.Progress = 60
	}
	if test.MonitorConf.Timeouts.Progress > test.MonitorConf.Timeouts.Prompt {
		return nil, errors.New("test161: progress timeout must be less than (or equal to) the prompt timeout")
	}
	if test.MonitorConf.Resolution == 0 {
		// Recording all statistics can lead to out of memory errors on long
		// tests, so we set a slower statistic interval when all stats are being
		// recorded. Otherwise use a smaller default for more precise timing.
		if test.MonitorConf.AllStats == "true" {
			test.MonitorConf.Resolution = 50000 // 50ms
		} else {
			test.MonitorConf.Resolution = 100 // 0.1ms
		}
	}
	if test.MonitorConf.Window == 0 {
		test.MonitorConf.Window = 2.0
	}
	if test.MonitorConf.Kernel.Min == 0.0 {
		test.MonitorConf.Kernel.Min = 0.001
	}
	if test.MonitorConf.Kernel.Min < 0.0 || test.MonitorConf.Kernel.Min > 1.0 {
		return nil, errors.New("test161: cycle limits must be fractions between 0.0 and 1.0")
	}
	if test.MonitorConf.Kernel.Max == 0.0 {
		test.MonitorConf.Kernel.Max = 0.99
	}
	if test.MonitorConf.Kernel.Max < 0.0 || test.MonitorConf.Kernel.Max > 1.0 {
		return nil, errors.New("test161: cycle limits must be fractions between 0.0 and 1.0")
	}
	if test.MonitorConf.Kernel.Min > test.MonitorConf.Kernel.Max {
		return nil, errors.New("test161: cycle minimum must be less than the maximum")
	}
	if test.MonitorConf.User.Min == 0.0 {
		test.MonitorConf.User.Min = 0.0001
	}
	if test.MonitorConf.User.Min < 0.0 || test.MonitorConf.User.Min > 1.0 {
		return nil, errors.New("test161: cycle limits must be fractions between 0.0 and 1.0")
	}
	if test.MonitorConf.User.Max == 0.0 {
		test.MonitorConf.User.Max = 1.0
	}
	if test.MonitorConf.User.Max < 0.0 || test.MonitorConf.User.Max > 1.0 {
		return nil, errors.New("test161: cycle limits must be fractions between 0.0 and 1.0")
	}
	if test.MonitorConf.User.Min > test.MonitorConf.User.Max {
		return nil, errors.New("test161: cycle minimum must be less than the maximum")
	}
	return test, err
}

// PrintConf formats the test configuration for use by sys161 via the sys161.conf file.
func (t *Test) PrintConf() (string, error) {
	const base = `0	serial
1	emufs{{if .Disk1.Sectors}}
2	disk	rpm={{.Disk1.RPM}}	file={{.Disk1.File}} {{if eq .Disk1.NoDoom "true"}}nodoom{{end}} # sectors={{.Disk1.Sectors}}{{end}}{{if .Disk2.Sectors}}
3	disk	rpm={{.Disk2.RPM}}	file={{.Disk2.File}} {{if eq .Disk2.NoDoom "true"}}nodoom{{end}} # sectors={{.Disk2.Sectors}}{{end}}
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
