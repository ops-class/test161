package test161

import (
	"github.com/ericaro/frontmatter"
	// "github.com/jamesharr/expect"
	// "gopkg.in/yaml.v2"
)

type Test struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags,flow"`
	Depends     []string `yaml:"depends,flow"`
	Content     string   `fm:"content" yaml:"-"`
}

func Load(data []byte) (*Test, error) {
	test := new(Test)
	err := frontmatter.Unmarshal(data, test)
	return test, err
}
