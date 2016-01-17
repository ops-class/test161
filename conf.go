package test161

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/ericaro/frontmatter"
	"github.com/imdario/mergo"
	"io/ioutil"
	"math/rand"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"text/template"
)

// Many of the values below that come in from YAML are string types. This
// allows us to work around Go not having a nil type for things like ints, and
// also allows us to accept values like "10M" as numbers. Non-string types are
// only used when the default doesn't make sense. Given that the ultimate
// destination of these values is strings (sys161.conf configuration or JSON
// output) it doesn't matter.

type Sys161Conf struct {
	CPUs   uint     `yaml:"cpus" json:"cpus"`
	RAM    string   `yaml:"ram" json:"ram"`
	Disk1  DiskConf `yaml:"disk1" json:"disk1"`
	Disk2  DiskConf `yaml:"disk2" json:"disk2"`
	Random uint32   `yaml:"-" json:"randomseed"`
}

type DiskConf struct {
	Enabled string `yaml:"enabled" json:"enabled"`
	RPM     uint   `yaml:"rpm" json:"rpm"`
	Bytes   string `yaml:"bytes" json:"bytes"`
	NoDoom  string `yaml:"nodoom" json:"nodoom"`
}

type StatConf struct {
	Resolution float32 `yaml:"resolution" json:"resolution"`
	Window     uint    `yaml:"window" json:"window"`
}

type MonitorConf struct {
	Enabled         string  `yaml:"enabled" json:"enabled"`
	Window          uint    `yaml:"window" json:"window"`
	Kernel          Limits  `yaml:"kernel" json:"kernel"`
	User            Limits  `yaml:"user" json:"user"`
	ProgressTimeout float32 `yaml:"progresstimeout" json:"progresstimeout"`
}

type Limits struct {
	EnableMin string  `yaml:"enablemin" json:"enablemin"`
	Min       float64 `yaml:"min" json:"min"`
	Max       float64 `yaml:"max" json:"max"`
}

type MiscConf struct {
	CommandRetries   uint    `yaml:"commandretries" json:"commandretries"`
	PromptTimeout    float32 `yaml:"prompttimeout" json:"prompttimeout"`
	CharacterTimeout uint    `yaml:"charactertimeout" json:"charactertimeout"`
	TempDir          string  `yaml:"tempdir" json:"-"`
	RetryCharacters  string  `yaml:"retrycharacters" json:"retrycharacters"`
	KillOnExit       string  `yaml:"killonexit" json:"killonexit"`
}

type CommandConf struct {
	Prefix string `yaml:"prefix" json:"prefix"`
	Prompt string `yaml:"prompt" json:"prompt"`
	Start  string `yaml:"start" json:"start"`
	End    string `yaml:"end" json:"end"`
}

var CONF_DEFAULTS = Test{
	Sys161: Sys161Conf{
		CPUs: 8,
		RAM:  "1M",
		Disk1: DiskConf{
			Enabled: "true",
			Bytes:   "4M",
			RPM:     7200,
			NoDoom:  "true",
		},
		Disk2: DiskConf{
			Enabled: "false",
			Bytes:   "2M",
			RPM:     7200,
			NoDoom:  "false",
		},
	},
	Stat: StatConf{
		Resolution: 0.01,
		Window:     1,
	},
	Monitor: MonitorConf{
		Enabled: "true",
		Window:  400,
		Kernel: Limits{
			EnableMin: "true",
			Min:       0.001,
			Max:       0.99,
		},
		User: Limits{
			EnableMin: "true",
			Min:       0.0001,
			Max:       1.0,
		},
		ProgressTimeout: 10.0,
	},
	Misc: MiscConf{
		CommandRetries:   5,
		PromptTimeout:    300.0,
		CharacterTimeout: 250,
		RetryCharacters:  "true",
		KillOnExit:       "false",
	},
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
	test.Sys161.Random = rand.Uint32() >> 16

	// TODO: Error checking here

	// Check for empty commands and expand syntatic sugar before getting
	// started. Doing this first makes the main loop and retry logic simpler.

	err = test.initCommands()
	if err != nil {
		return nil, err
	}

	return test, nil
}

func (t *Test) MergeConf(defaults Test) error {
	return mergo.Map(t, defaults)
}

const SYS161_TEMPLATE = `0 serial
1	emufs
{{if eq .Disk1.Enabled "true"}}
2	disk rpm={{.Disk1.RPM}} file=LHD0.img {{if eq .Disk1.NoDoom "true"}}nodoom{{end}} # bytes={{.Disk1.Bytes }}
{{end}}
{{if eq .Disk2.Enabled "true"}}
3	disk rpm={{.Disk2.RPM}} file=LHD1.img {{if eq .Disk2.NoDoom "true"}}nodoom{{end}} # bytes={{.Disk2.Bytes }}
{{end}}
28	random seed={{.Random}}
29	timer
30	trace
31	mainboard ramsize={{.RAM}} cpus={{.CPUs}}`

var confTemplate *template.Template
var confOnce sync.Once

// PrintConf formats the test configuration for use by sys161 via the sys161.conf file.
func (t *Test) PrintConf() (string, error) {

	confOnce.Do(func() { confTemplate, _ = template.New("conf").Parse(SYS161_TEMPLATE) })

	buffer := new(bytes.Buffer)

	// Can't fail, even if the test isn't properly initialized, because
	// t.Sys161 matches the template
	confTemplate.Execute(buffer, t.Sys161)

	var confString string
	for _, line := range strings.Split(strings.TrimSpace(buffer.String()), "\n") {
		if strings.TrimSpace(line) != "" {
			confString += line + "\n"
		}
	}
	return confString, nil
}

func (t *Test) confEqual(t2 *Test) bool {
	return t.Sys161 == t2.Sys161 &&
		t.Stat == t2.Stat &&
		t.Monitor == t2.Monitor &&
		t.Misc == t2.Misc &&
		reflect.DeepEqual(t.CommandConf, t2.CommandConf)
}

var prefixRegexp *regexp.Regexp
var prefixOnce sync.Once

func (t *Test) checkCommandConf() error {
	prefixOnce.Do(func() { prefixRegexp = regexp.MustCompile(`^([!@#$%^&*]) `) })

	for _, commandConf := range t.CommandConf {
		if commandConf.Prefix == "" || commandConf.Prompt == "" ||
			commandConf.Start == "" || commandConf.End == "" {
			return errors.New(fmt.Sprintf("test161: need to specific command prefix, prompt, start, and end"))
		}
		if len(commandConf.Prefix) > 1 {
			return errors.New(fmt.Sprintf("test161: illegal multicharacter prefix %v", commandConf.Prefix))
		}
		matches := prefixRegexp.FindStringSubmatch(commandConf.Prefix + " ")
		if len(matches) == 0 {
			return errors.New(fmt.Sprintf("test161: found invalid prefix %v", commandConf.Prefix))
		}
		if matches[1] == "$" {
			return errors.New(fmt.Sprintf("test161: the $ prefix is reserved for the shell"))
		}
	}
	return nil
}

var KERNEL_COMMAND_CONF = &CommandConf{
	Prompt: KERNEL_PROMPT,
	End:    "q",
}
var SHELL_COMMAND_CONF = &CommandConf{
	Prompt: `OS/161$ `,
	Start:  "s",
	End:    "exit",
}

const KERNEL_PROMPT = `OS/161 kernel [? for menu]: `

func (t *Test) commandConfFromLine(commandLine string) (string, *CommandConf) {
	commandLine = strings.TrimSpace(commandLine)
	matches := prefixRegexp.FindStringSubmatch(commandLine)
	if len(matches) == 0 {
		return commandLine, KERNEL_COMMAND_CONF
	} else if matches[1] == "$" {
		return commandLine[2:], SHELL_COMMAND_CONF
	} else {
		for _, commandConf := range t.CommandConf {
			if commandConf.Prefix == matches[1] {
				return commandLine[2:], &commandConf
			}
		}
	}
	return commandLine, nil
}

func (t *Test) initCommands() (err error) {

	// Check defined command prefixes
	err = t.checkCommandConf()
	if err != nil {
		return err
	}

	// Set up the command configuration stack
	var commandConfStack []*CommandConf
	commandConfStack = append(commandConfStack, KERNEL_COMMAND_CONF)

	// Set the boot command
	t.Commands = append(t.Commands, Command{
		Type:   "kernel",
		Prompt: KERNEL_PROMPT,
		Input: InputLine{
			Line: "boot",
		},
	})

	commandLines := strings.Split(strings.TrimSpace(t.Content), "\n")
	counter = 0
	for {
		commandLine = commandLines[counter]
		commandLine = strings.TrimSpace(commandLine)
		if commandLine == "" {
			return errors.New("test161: found empty command")
		}

		commandLine, commandConf := t.commandConfFromLine(commandLine)
		if commandConf == nil {
			return errors.New("test161: command with invalid prefix")
		}

		currentConf := commandConfStack[len(commandConfStack)-1]
		if currentConf != commandConf {
			var foundPrevious
			var exitStack []string
			for i = len(commandConfStack) - 1; i >= 0; i++ {
				if currentConf == commandConfStack[i] {
					foundPrevious = true
					break
				} else {
					exitStack = append(exitStack, commandConfStack[i].Exit)
				}
			}
			// Get from point a to point b
		}
		t.Commands = append(t.Commands, Command{
			Input: InputLine{
				Line: commandLine,
			},
		})
	}

	return nil
}
