package test161

import (
	"bytes"
	//"errors"
	"github.com/ericaro/frontmatter"
	//"github.com/gchallen/expect"
	"github.com/imdario/mergo"
	"io/ioutil"
	"math/rand"
	//"regexp"
	//"strconv"
	"strings"
	//"sync"
	"text/template"
	//"unicode"
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
	Resolution float32 `yaml:"interval" json:"interval"`
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
	Min float64 `yaml:"min" json:"min"`
	Max float64 `yaml:"max" json:"max"`
}

type MiscConf struct {
	CommandRetries uint    `yaml:"commandretries" json:"commandretries"`
	PromptTimeout  float32 `yaml:"prompttimeout" json:"prompttimeout"`
}

var CONF_DEFAULTS = Test{
	Sys161: Sys161Conf{
		CPUs: 8,
		RAM:  "1M",
		Disk1: DiskConf{
			Enabled: "true",
			RPM:     7200,
			Bytes:   "2M",
			NoDoom:  "true",
		},
		Disk2: DiskConf{
			Enabled: "false",
			RPM:     7200,
			Bytes:   "2M",
			NoDoom:  "false",
		},
	},
	Stat: StatConf{
		Resolution: 0.01,
		Window:     1,
	},
	Monitor: MonitorConf{
		Enabled: "true",
		Window:  10,
		Kernel: Limits{
			Min: 0.001,
			Max: 0.99,
		},
		User: Limits{
			Min: 0.0001,
			Max: 1.0,
		},
		ProgressTimeout: 10.0,
	},
	Misc: MiscConf{
		CommandRetries: 5,
	},
}

// TestFromFile parses the test file and sets configuration defaults.
func TestFromFile(filename string, defaults *Test) (*Test, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return TestFromString(string(data), defaults)
}

// TestFromFile parses the test string and sets configuration defaults.
func TestFromString(data string, defaults *Test) (*Test, error) {
	test := new(Test)

	err := frontmatter.Unmarshal([]byte(data), test)
	if err != nil {
		return nil, err
	}

	if defaults != nil {
		err := mergo.Map(&test, defaults)
		if err != nil {
			return nil, err
		}
	}

	err = mergo.Map(&test, CONF_DEFAULTS)
	if err != nil {
		return nil, err
	}

	test.Sys161.Random = rand.Uint32() >> 16

	return test, err
}

const SYS161_TEMPLATE = `0	serial
1	emufs
{{if eq .Disk1.Enabled "true"}}
2	disk rpm={{.Disk1.RPM}} file=LHD0.img {{if eq .Disk1.NoDoom "true"}}nodoom{{end}} # bytes={{.Disk1.Bytes }}
{{end}}
{{if eq .Disk1.Enabled "true"}}
3	disk rpm={{.Disk2.RPM}} file=LHD1.img {{if eq .Disk2.NoDoom "true"}}nodoom{{end}} # bytes={{.Disk2.Bytes }}
{{end}}
28	random seed={{.Random}}
29	timer
30	trace
31	mainboard ramsize={{.RAM}} cpus={{.CPUs}}`

// PrintConf formats the test configuration for use by sys161 via the sys161.conf file.
func (t *Test) PrintConf() (string, error) {

	conf, err := template.New("conf").Parse(SYS161_TEMPLATE)
	if err != nil {
		return "", err
	}
	buffer := new(bytes.Buffer)
	err = conf.Execute(buffer, t.Sys161)
	if err != nil {
		return "", err
	}
	var confString string
	for _, line := range strings.Split(strings.TrimSpace(buffer.String()), "\n") {
		if strings.TrimSpace(line) != "" {
			confString += line
		}
	}
	return confString, nil
}
