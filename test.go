package test161

import (
	"bytes"
	"errors"
	//"fmt"
	"github.com/ericaro/frontmatter"
	"github.com/termie/go-shutil"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"text/template"
	"unicode"
	// "github.com/jamesharr/expect"
	// "gopkg.in/yaml.v2"
)

type DiskConfig struct {
	Defaults bool   `yaml:"defaults"`
	RPM      uint   `yaml:"rpm"`
	Sectors  string `yaml:"sectors"`
	Bytes    string
	NoDoom   string `yaml:"nodoom"`
	File     string
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

func convertNumber(in string, unit int) (string, error) {
	if in == "" {
		return "", nil
	}
	if unit == 0 {
		unit = 1
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

	if test.Conf.CPUs == "" {
		test.Conf.CPUs = "1"
	}
	if test.Conf.RAM == "" {
		test.Conf.RAM = "1M"
	}
	if test.Conf.Disk1.RPM == 0 {
		test.Conf.Disk1.RPM = 7200
	}
	if test.Conf.Disk1.Sectors == "" {
		test.Conf.Disk1.Sectors = "5M"
	}
	if test.Conf.Disk1.NoDoom == "" {
		test.Conf.Disk1.NoDoom = "true"
	}
	test.Conf.Disk1.File = "LDH0.img"

	test.Conf.RAM, err = convertNumber(test.Conf.RAM, 1)
	if err != nil {
		return nil, err
	}

	test.Conf.Disk1.Bytes, err = convertNumber(test.Conf.Disk1.Sectors, 1)
	if err != nil {
		return nil, err
	}
	test.Conf.Disk1.Sectors, err = convertNumber(test.Conf.Disk1.Bytes, 512)
	if err != nil {
		return nil, err
	}
	if test.Conf.Disk1.NoDoom == "false" {
		test.Conf.Disk1.NoDoom = ""
	}

	if test.Conf.Disk2.Sectors != "" {
		test.Conf.Disk2.Bytes, err = convertNumber(test.Conf.Disk2.Sectors, 1)
		if err != nil {
			return nil, err
		}
		test.Conf.Disk2.Sectors, err = convertNumber(test.Conf.Disk2.Bytes, 512)
		if err != nil {
			return nil, err
		}
		if test.Conf.Disk2.RPM == 0 {
			test.Conf.Disk2.RPM = 7200
		}
		if test.Conf.Disk2.NoDoom == "" {
			test.Conf.Disk2.NoDoom = "false"
		}
		if test.Conf.Disk2.NoDoom == "false" {
			test.Conf.Disk2.NoDoom = ""
		}
		test.Conf.Disk2.File = "LDH1.img"
	}

	if test.Conf.Random == "" {
		test.Conf.Random = strconv.Itoa(int(rand.Int31() >> 16))
	}
	return test, err
}

func (t *Test) Run(kernel string, root string, tempRoot string) error {
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

	kernelTarget := path.Join(tempDir, "kernel")
	if kernel != "" {
		_, err = shutil.Copy(kernel, kernelTarget, true)
		if err != nil {
			return err
		}
	}
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

	return nil
}

// PrintConf formats the test configuration for use by sys161 through the sys161.conf file.
func (t *Test) PrintConf() (string, error) {
	const base = `0	serial
1	emufs{{if .Disk1.Sectors}}
2	disk	rpm={{.Disk1.RPM}}	sectors={{.Disk1.Sectors}}	file={{ .Disk1.File}} {{if .Disk1.NoDoom}}nodoom{{end}}{{end}}{{if .Disk2.Sectors}}
3	disk	rpm={{.Disk2.RPM}}	sectors={{.Disk2.Sectors}}	file={{ .Disk2.File}} {{if .Disk2.NoDoom}}nodoom{{end}}{{end}}
28	random	{{.Random}}
29	timer
30	trace
31	mainboard  ramsize={{.RAM}}  cpus={{.CPUs}}`

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
