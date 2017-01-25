package test161

import (
	"fmt"
	yaml "gopkg.in/yaml.v2"
	"io/ioutil"
)

type TagDescription struct {
	Name        string `yaml:"name"`
	Description string `yaml:"desc"`
}

// TagDescription Collection. We just use this for loading and move the
// references into a map in the global environment.
type TagDescriptions struct {
	Tags []*TagDescription `yaml:"tags"`
}

func TagDescriptionsFromFile(file string) (*TagDescriptions, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("Error reading tags file %v: %v", file, err)
	}

	tags, err := TagDescriptionsFromString(string(data))
	if err != nil {
		err = fmt.Errorf("Error loading tags file %v: %v", file, err)
	}
	return tags, err
}

func TagDescriptionsFromString(text string) (*TagDescriptions, error) {
	tags := &TagDescriptions{}
	err := yaml.Unmarshal([]byte(text), tags)

	if err != nil {
		return nil, err
	}

	return tags, nil
}
