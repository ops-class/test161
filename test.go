package test161

import (
	"errors"
	"github.com/ericaro/frontmatter"
	"github.com/termie/go-shutil"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"unicode"
	// "github.com/jamesharr/expect"
	// "gopkg.in/yaml.v2"
)

type Disk struct {
	Sectors string `yaml:"sectors"`
	NoDoom  bool   `yaml:"nodoom"`
}

type Conf struct {
	CPUs  string `yaml:"cpus"` // Default 1
	RAM   string `yaml:"ram"`
	Disk1 Disk   `yaml:"disk1"`
	Disk2 Disk   `yaml:"disk2"`
}

type Test struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
	Depends     []string `yaml:"depends"`
	Config      Conf     `yaml:"conf"`
	Content     string   `fm:"content" yaml:"-"`
}

func LoadTest(filename string) (*Test, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	test := new(Test)
	err = frontmatter.Unmarshal(data, test)
	if test.Config.RAM != "" {
		if !unicode.IsDigit(rune(test.Config.RAM[len(test.Config.RAM)-1])) {
			number, err := strconv.Atoi(test.Config.RAM[0 : len(test.Config.RAM)-1])
			if err != nil {
				return nil, err
			}
			multiplier := strings.ToUpper(string(test.Config.RAM[len(test.Config.RAM)-1]))
			if multiplier == "K" {
				test.Config.RAM = strconv.Itoa(1024 * number)
			} else if multiplier == "M" {
				test.Config.RAM = strconv.Itoa(1024 * 1024 * number)
			} else {
				return nil, errors.New("test161: could not convert RAM to integer")
			}
		}
	}
	if test.Config.CPUs == "" {
		test.Config.CPUs = "1"
	}
	if test.Config.RAM == "" {
		test.Config.RAM = strconv.Itoa(1024 * 1024)
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

/*
func (*Test) Conf() error {
	base := `
0	serial
1	emufs
{{if .Conf.Disk1}}2	disk	rpm=7200	sectors={{.Conf.Disk1.Sectors}}	file=LHD0.img {{if .Conf.Disk1.NoDoom}}nodoom{{end}}{{end}}
{{if .Conf.Disk2}}3	disk	rpm=7200	sectors={{.Conf.Disk2.Sectors}}	file=LHD0.img {{if .Conf.Disk2.NoDoom}}nodoom{{end}}{{end}}
28	random	autoseed
29	timer
30	trace
31	mainboard  ramsize={{.Conf.RAM}}  cpus={{.Conf.CPUs}}
`
	return nil
}
*/
