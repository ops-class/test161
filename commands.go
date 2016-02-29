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
// template, with various functions provided.  Furthermore, random inputs can also be
// generated using a template, which can be overriden in assignment files if needed.
//
// Command Instances: The input/expected output for an instance of a command instance
// is created by executing the templates.
//
// Composite Commands: Some commands may execute other commands, i.e. triple*.  For
// these cases, there is an "external" property of the output line, which when set to
// "true", specifies that the text property should be used to look up the output from
// another command.
//

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
	temp := rand.Intn(max - min)

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

// Functions we provide to the command templates.
var funcMap template.FuncMap = template.FuncMap{
	"add":        add,
	"atoi":       atoi,
	"factorial":  factorial,
	"randInt":    randInt,
	"randString": randString,
	"ranger":     ranger,
}

// Data that we provide for command templates.
type templateData struct {
	Args   []string
	ArgLen int
}

// Command options for panics and timesout
const (
	CMD_OPT_NO    = "no"
	CMD_OPT_MAYBE = "maybe"
	CMD_OPT_YES   = "yes"
)

// Template for commands instances.  These get expanded depending on the command environment.
type CommandTemplate struct {
	Name     string             `yaml:"name"`
	Output   []*TemplOutputLine `yaml:"output"`
	Input    []string           `yaml:"input"`
	Panic    string             `yaml:"panics"`   // CMD_OPT
	TimesOut string             `yaml:"timesout"` // CMD_OPT
	Timeout  float32            `yaml:"timeout"`  // Timeout in sec. A timeout of 0.0 uses the test default.
}

// An expected line of output, which may either be expanded or not.
type TemplOutputLine struct {
	Text     string `yaml:"text"`
	Trusted  string `yaml:"trusted"`
	External string `yaml:"external"`
}

// Command instance expected output line.  The difference here is that we store the name
// of the key that we need to verify the output.
type ExpectedOutputLine struct {
	Text    string
	Trusted bool
	KeyName string
}

// Expand the golang text template using the provided tempate data.
// We do this on a per-command instance basis, since output can change
// depending on input.
func expandLine(t string, templdata interface{}) ([]string, error) {

	res := make([]string, 0)
	bb := &bytes.Buffer{}

	if tmpl, err := template.New("CommandInstance").Funcs(funcMap).Parse(t); err != nil {
		return nil, err
	} else if tmpl.Execute(bb, templdata); err != nil {
		return nil, err
	} else {
		lines := strings.Split(bb.String(), "\n")
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				res = append(res, l)
			}
		}
	}

	return res, nil
}

// Expand template output lines into the actual expected output. This may be
// called recursively if the output line references another command.  The
// 'processed' map takes care of checking for cycles so we don't get stuck.
func expandOutput(id string, td *templateData, processed map[string]bool, env *TestEnvironment) ([]*ExpectedOutputLine, error) {

	var tmpl *CommandTemplate
	var ok bool

	// Check for cycles
	if _, ok = processed[id]; ok {
		return nil, errors.New("Cycle detected in command template output.  ID: " + id)
	}
	processed[id] = true

	// Get the template
	tmpl, ok = env.Commands[id]
	if !ok {
		return nil, errors.New("Cannot find " + id + "in command map")
	}

	// expected output
	expected := make([]*ExpectedOutputLine, 0)

	// Expand each expected output line, possibly referencing external commands
	for _, origline := range tmpl.Output {
		if origline.External == "true" {
			if more, err := expandOutput(origline.Text, td, processed, env); err != nil {
				return nil, err
			} else {
				expected = append(expected, more...)
			}
		} else {
			if lines, err := expandLine(origline.Text, td); err != nil {
				return nil, err
			} else {
				for _, expandedline := range lines {
					expectedline := &ExpectedOutputLine{
						Text: expandedline,
					}
					if origline.Trusted == "true" {
						expectedline.Trusted = true
						expectedline.KeyName = id
					} else {
						expectedline.Trusted = false
						expectedline.KeyName = ""
					}
					expected = append(expected, expectedline)
				}
			}
		}
	}

	return expected, nil
}

func (c *Command) Id() string {
	_, id, _ := (&c.Input).splitCommand()
	return id
}

// Instantiate the command (input, expected output) using the command template.
// This needs to be must be done prior to executing the command.
func (c *Command) Instantiate(env *TestEnvironment) error {
	pfx, id, args := (&c.Input).splitCommand()
	tmpl, ok := env.Commands[id]
	if !ok {
		// OK, it's just not a command that has any input/output specification.
		return nil
	}

	c.Panic = tmpl.Panic
	c.TimesOut = tmpl.TimesOut
	c.Timeout = tmpl.Timeout

	// Input

	// Check if  we need to create some input. If args haven't already been
	// specified, and there is an input template, use that to create input.
	if len(args) == 0 && len(tmpl.Input) > 0 {
		args = make([]string, 0)
		for _, line := range tmpl.Input {
			if temp, err := expandLine(line, "No data"); err != nil {
				return err
			} else {
				args = append(args, temp...)
			}
		}
	}

	// Output

	// template data for the output
	td := &templateData{args, len(args)}
	processed := make(map[string]bool)

	if expected, err := expandOutput(id, td, processed, env); err != nil {
		return err
	} else {
		// Piece back together a command line for the command
		commandLine := ""

		if len(pfx) > 0 {
			commandLine += pfx + " "
		}

		commandLine += id

		for _, arg := range args {
			commandLine += " " + arg
		}

		c.Input.Line = commandLine
		c.ExpectedOutput = expected

		return nil
	}

}

// CommandTemplate Collection. We just use this for loading and move the
// references into a map in the global environment.
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

	for _, t := range cmds.Templates {
		t.fixDefaults()
	}

	return cmds, nil
}

func (t *CommandTemplate) fixDefaults() {

	// Fix poor default value support in go-yaml/golang.
	//
	// If we find an empty output line, delete it - these are commands
	// that do not expect output. If we find a command with no expected
	// output, add the default expected output.

	// Default for panic is to not allow it, i.e. must return to prompt
	if t.Panic != CMD_OPT_MAYBE && t.Panic != CMD_OPT_YES {
		t.Panic = CMD_OPT_NO
	}

	if t.TimesOut != CMD_OPT_MAYBE && t.TimesOut != CMD_OPT_YES {
		t.TimesOut = CMD_OPT_NO
	}

	if len(t.Output) == 1 && strings.TrimSpace(t.Output[0].Text) == "" {
		t.Output = nil
	} else if len(t.Output) == 0 {
		t.Output = []*TemplOutputLine{
			&TemplOutputLine{
				Trusted:  "true",
				External: "false",
				Text:     t.Name + ": SUCCESS",
			},
		}
	} else {
		for _, line := range t.Output {
			if line.Trusted != "false" {
				line.Trusted = "true"
			}

			if line.External != "true" {
				line.External = "false"
			}
		}
	}
}
