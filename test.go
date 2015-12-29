package test161

import (
	"errors"
	"fmt"
	"github.com/ericaro/frontmatter"
	"github.com/termie/go-shutil"
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"text/template"
	"unicode"
	// "github.com/jamesharr/expect"
	// "gopkg.in/yaml.v2"
)

type DiskConfig struct {
	Sectors string `yaml:"sectors"`
	NoDoom  bool   `yaml:"nodoom"`
}

type Config struct {
	CPUs   string     `yaml:"cpus"` // Default 1
	RAM    string     `yaml:"ram"`
	Random string     `yaml:"random"`
	Disk1  DiskConfig `yaml:"disk1"`
	Disk2  DiskConfig `yaml:"disk2"`
}

type Test struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
	Depends     []string `yaml:"depends"`
	Conf        Config   `yaml:"conf"`
	Content     string   `fm:"content" yaml:"-"`
}

func LoadTest(filename string) (*Test, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	test := new(Test)
	err = frontmatter.Unmarshal(data, test)
	if test.Conf.RAM != "" {
		if !unicode.IsDigit(rune(test.Conf.RAM[len(test.Conf.RAM)-1])) {
			number, err := strconv.Atoi(test.Conf.RAM[0 : len(test.Conf.RAM)-1])
			if err != nil {
				return nil, err
			}
			multiplier := strings.ToUpper(string(test.Conf.RAM[len(test.Conf.RAM)-1]))
			if multiplier == "K" {
				test.Conf.RAM = strconv.Itoa(1024 * number)
			} else if multiplier == "M" {
				test.Conf.RAM = strconv.Itoa(1024 * 1024 * number)
			} else {
				return nil, errors.New("test161: could not convert RAM to integer")
			}
		}
	}
	if test.Conf.CPUs == "" {
		test.Conf.CPUs = "1"
	}
	if test.Conf.RAM == "" {
		test.Conf.RAM = strconv.Itoa(1024 * 1024)
	}
	if test.Conf.Random == "" {
		fmt.Println(rand.Int31())
		test.Conf.Random = strconv.Itoa(int(rand.Int31() >> 16))
	}
	return test, err
}

func (*Test) Run(kernel string, root string, tempRoot string) error {
	tempDir, err := ioutil.TempDir(tempRoot, "test161")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	if root != "" {
		err = shutil.CopyTree(root, tempDir, nil)
		if err != nil {
			return err
		}
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return err
	}
	defer os.Chdir(currentDir)
	os.Chdir(tempDir)

	return nil
}

func (t *Test) PrintConf() error {
	const base = `0	serial
1	emufs{{if .Disk1.Sectors}}
2	disk	rpm=7200	sectors={{.Disk1.Sectors}}	file=LHD0.img {{if .Disk1.NoDoom}}nodoom{{end}}{{end}}{{if .Disk2.Sectors}}
3	disk	rpm=7200	sectors={{.Disk2.Sectors}}	file=LHD0.img {{if .Disk2.NoDoom}}nodoom{{end}}{{end}}
28	random	{{.Random}}
29	timer
30	trace
31	mainboard  ramsize={{.RAM}}  cpus={{.CPUs}}
`

	conf, err := template.New("conf").Parse(base)
	if err != nil {
		return err
	}
	err = conf.Execute(os.Stdout, t.Conf)

	return nil
}
