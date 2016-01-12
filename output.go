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
	if string(output[len(output)-1]) != "\n" {
		output += "\n"
	}
	output += fmt.Sprintf("%.6f\t%s", t.SimTime, t.Status)
	if t.ShutdownMessage != "" {
		output += fmt.Sprintf(": %s", t.ShutdownMessage)
	}
	output += "\n"
	return output
}
