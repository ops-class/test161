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

type Conf struct {
	CPUs   uint8  `yaml:"cpus"`
	RAM    string `yaml:"ram"`
	Memory uint32
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
		if unicode.IsDigit(rune(test.Config.RAM[len(test.Config.RAM)-1])) {
			RAM, err := strconv.Atoi(test.Config.RAM)
			if err != nil {
				return nil, err
			}
			test.Config.Memory = uint32(RAM)
		} else {
			number, err := strconv.Atoi(test.Config.RAM[0 : len(test.Config.RAM)-1])
			if err != nil {
				return nil, err
			}
			multiplier := strings.ToUpper(string(test.Config.RAM[len(test.Config.RAM)-1]))
			if multiplier == "K" {
				test.Config.Memory = uint32(1024 * number)
			} else if multiplier == "M" {
				test.Config.Memory = uint32(1024 * 1024 * number)
			} else {
				return nil, errors.New("test161: could not convert RAM to integer")
			}
		}
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
