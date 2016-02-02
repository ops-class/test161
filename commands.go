package test161

import (
	"bytes"
	"errors"
	yaml "gopkg.in/yaml.v2"
	"io/ioutil"
	"math/rand"
	"strconv"
	"strings"
	"text/template"
)

// This file has everything for creating command instances from command templates.
// In test161, a command is either a command run from the kernel menu (sy1), or
// a single userspace program (i.e. /testbin/huge).
//
// Command Templates: Some commands, (e.g. argtest, factorial), have output that
// depends on the input.  For this reason, output may be specified as a golang
// template, with various functions provide.  Furthermore, random inputs can also be
// specifed using a template, which can be overriden in assignment files.
//
// Command Instances: A command instance is created by executing the input/output
// templates.
//
// Composite Commands: Some commands may execute other commands, i.e. triple*.  For
// these cases, there is an external property of the output line, which when set to
// "true", specifies that the text property should be used to look up the output from
// another command.
//

// templateData gets passed to the command template to create a command instance.
type templateData struct {
	Args   []string
	ArgLen int
	Vars   []string
}

// Functions and function map & functions for command template evaluation

func add(a int, b int) int {
	return a + b
}

func atoi(s string) (int, error) {
	return strconv.Atoi(s)
}

func randInt(min, max int) (int, error) {
	if min >= max {
		return 0, errors.New("max must be greater than min")
	}

	// between 0 and max-min
	temp := rand.Intn((max + 1) - min)

	// between min and mix
	return min + temp, nil
}

const stringChars string = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()"

func randString(min, max int) (string, error) {
	// Get the length of the string
	l, err := randInt(min, max)
	if err != nil {
		return "", err
	}

	// Make it
	b := make([]byte, l)
	for i := 0; i < l; i++ {
		b[i] = stringChars[rand.Intn(len(stringChars))]
	}

	return string(b), nil
}

func factorial(n int) int {
	if n == 0 {
		return 1
	} else {
		return n * factorial(n-1)
	}
}

// Get something of length n that we can range over.  This is useful
// for creating random inputs.
func ranger(n int) []int {
	res := make([]int, n)
	for i := 0; i < n; i++ {
		res[i] = i
	}
	return res
}

// All the functions we provide to the command templates.
var funcMap template.FuncMap = template.FuncMap{
	"add":        add,
	"atoi":       atoi,
	"factorial":  factorial,
	"randInt":    randInt,
	"randString": randString,
	"ranger":     ranger,
}

// Template for commands.  These get expanded depending on the command environment.
type CommandTemplate struct {
	Name   string            `yaml:"name"`
	Output []TemplOutputLine `yaml:"output"`
	Input  []string          `yaml:"input"`
}

// An expected line of output, which may either be expanded or not.
type TemplOutputLine struct {
	Text     string `yaml:"text"`
	Trusted  string `yaml:"trusted"`
	External string `yaml:"external"`
}

// CommandInstance is the result of evaluating a CommandTemplate
type CommandInstance struct {
	CommandName    string
	ShortName      string
	Args           []string
	ExpectedOutput []*InstOutputLine
}

// Command Instance output line.  The difference here is that we store the name
// of the key that we need to verify the output.
type InstOutputLine struct {
	Text    string
	Trusted bool
	KeyName string
}

type expandedLine struct {
	line    string
	origPos int
}

func expandTemplates(templates []string, templdata interface{}) ([]*expandedLine, error) {

	res := make([]*expandedLine, 0)

	for pos, t := range templates {

		bb := &bytes.Buffer{}

		if tmpl, err := template.New("input").Funcs(funcMap).Parse(t); err != nil {
			return nil, err
		} else if tmpl.Execute(bb, templdata); err != nil {
			return nil, err
		} else {
			lines := strings.Split(bb.String(), "\n")
			for _, l := range lines {
				if strings.TrimSpace(l) != "" {
					res = append(res, &expandedLine{l, pos})
				}
			}
		}
	}

	return res, nil
}

func (c *CommandTemplate) expand(args, vars []string) (*CommandInstance, error) {

	// See if  we need to create some input
	if len(args) == 0 && len(c.Input) > 0 {
		if a, err := expandTemplates(c.Input, "No data"); err != nil {
			return nil, err
		} else {
			args = make([]string, 0, len(a))
			for _, l := range a {
				args = append(args, l.line)
			}
		}
	}

	// template data for the output
	td := &templateData{args, len(args), vars}

	lines := make([]string, len(c.Output))
	for i := 0; i < len(c.Output); i++ {
		lines[i] = c.Output[i].Text
	}

	output, err := expandTemplates(lines, td)
	if err != nil {
		return nil, err
	}

	cmd := &CommandInstance{}

	// Name
	cmd.CommandName = c.Name
	if pos := strings.LastIndex(c.Name, "/"); pos >= 0 {
		cmd.ShortName = c.Name[pos+1:]
	} else {
		cmd.ShortName = c.Name
	}

	// Args
	cmd.Args = args

	// Output
	cmd.ExpectedOutput = make([]*InstOutputLine, 0, len(output))
	for _, l := range output {
		expLine := &InstOutputLine{}
		expLine.Text = l.line
		if c.Output[l.origPos].Trusted == "true" {
			expLine.Trusted = true
		} else {
			expLine.Trusted = false
		}

		// If the original output line was external,
		if c.Output[l.origPos].External == "true" {
			expLine.KeyName = c.Output[l.origPos].Text
		} else {
			expLine.KeyName = cmd.ShortName
		}

		// TODO: Implement external commands

		cmd.ExpectedOutput = append(cmd.ExpectedOutput, expLine)
	}

	return cmd, nil
}

// CommandTemplate Collection.
// TODO: We should probably create a command template map so we
//       can look things up.
type CommandTemplates struct {
	Templates []*CommandTemplate `yaml:"templates"`
}

func CommandTemplatesFromFile(file string) (*CommandTemplates, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	return CommandTemplatesFromString(string(data))
}

func CommandTemplatesFromString(text string) (*CommandTemplates, error) {
	cmds := &CommandTemplates{}
	err := yaml.Unmarshal([]byte(text), cmds)

	if err != nil {
		return nil, err
	}

	return cmds, nil
}
