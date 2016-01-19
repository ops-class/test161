package test161

import (
	"encoding/json"
	"fmt"
	"strings"
)

// OutputJSON serializes the test object and all related output.
func (t *Test) OutputJSON() (string, error) {
	outputBytes, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return "", err
	}
	return string(outputBytes), nil
}

// OutputString prints test output in a human readable form.
func (t *Test) OutputString() string {
	var output string
	for _, conf := range strings.Split(t.ConfString, "\n") {
		conf = strings.TrimSpace(conf)
		output += fmt.Sprintf("conf: %s\n", conf)
	}
	for i, command := range t.Commands {
		for j, outputLine := range command.Output {
			if i == 0 || j != 0 {
				output += fmt.Sprintf("%.6f\t%s", outputLine.SimTime, outputLine.Line)
			} else {
				output += fmt.Sprintf("%s", outputLine.Line)
			}
		}
	}
	if len(output) > 0 && string(output[len(output)-1]) != "\n" {
		output += "\n"
	}
	if len(t.Status) > 0 {
		status := t.Status[len(t.Status)-1]
		output += fmt.Sprintf("%.6f\t%s", t.SimTime, status.Status)
		if status.Message != "" {
			output += fmt.Sprintf(": %s", status.Message)
		}
		output += "\n"
	}
	return output
}

func (tg *TestGroup) OutputJSON() (string, error) {
	outputBytes, err := json.MarshalIndent(tg, "", "  ")
	if err != nil {
		return "", err
	}
	return string(outputBytes), nil
}

func (tg *TestGroup) OutputString() string {
	var output string
	output = fmt.Sprintf("\ngroup: id      = %v\n", tg.Id())
	output += fmt.Sprintf("group: name    = %v\n", tg.Config.Name)
	output += fmt.Sprintf("group: rootdir = %v\n", tg.Config.RootDir)
	output += fmt.Sprintf("group: usedeps = %v\n", tg.Config.UseDeps)
	output += fmt.Sprintf("group: tags    = %v\n", tg.Config.Tags)

	for _, test := range tg.Tests {
		output += "\n"
		output += test.OutputString()
	}

	return output
}
