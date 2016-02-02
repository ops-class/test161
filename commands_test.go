package test161

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

// /testbin/add (output(
const add_template = "{{$x:= index .Args 0 | atoi}}{{$y := index .Args 1 | atoi}}{{add $x $y}}\n"

// /testbin/factorial (output)
const fact_template = "{{$n:= index .Args 0 | atoi}}{{factorial $n}}\n"

// A random string input generator.  This generates between 2 and 10
// strings, each of length between 5 and 10 characters.
const input_template = "{{$x := randInt 2 10 | ranger}}{{range $index, $element := $x}}{{randString 5 10}}\n{{end}}"

// /testbin/argtest (output)
var args_template = []string{
	"argc: {{add 1 .ArgLen}}",
	"argv[0]: /testbin/argtest",
	"{{range $index, $element := .Args}}argv[{{add $index 1}}]: {{$element}}\n{{end}}",
	"argv[{{add 1 .ArgLen}}]: [NULL]",
}

func TestCommand1(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	// Create the Command Template
	cc := &CommandTemplate{
		Name: "/testbin/argtest",
	}

	cc.Output = make([]TemplOutputLine, 0)
	for _, line := range args_template {
		cc.Output = append(cc.Output, TemplOutputLine{line, "true", "false"})
	}

	// Create the command instance
	args := []string{"arg1", "arg2", "arg3", "arg4"}
	vars := []string{}

	cmd, err := cc.expand(args, vars)
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	// Log the output

	t.Log(cmd.CommandName)
	t.Log(cmd.ShortName)
	t.Log(cmd.Args)

	for _, o := range cmd.ExpectedOutput {
		t.Log(o.Text)
	}

	// Assertions

	assert.Equal("/testbin/argtest", cmd.CommandName)
	assert.Equal("argtest", cmd.ShortName)

	assert.Equal(3+len(args), len(cmd.ExpectedOutput))

	if len(cmd.ExpectedOutput) == 7 {
		assert.Equal("argc: 5", cmd.ExpectedOutput[0].Text)
		assert.Equal("argv[0]: /testbin/argtest", cmd.ExpectedOutput[1].Text)
		for i, arg := range args {
			assert.Equal(fmt.Sprintf("argv[%d]: %v", i+1, arg), cmd.ExpectedOutput[i+2].Text)
		}
	}

}

func TestCommand2(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	// Create the Command Template
	cc := &CommandTemplate{
		Name:   "/testbin/add",
		Output: []TemplOutputLine{TemplOutputLine{add_template, "true", "false"}},
	}

	// Create the command instance
	args := []string{"70", "200"}
	vars := []string{}

	cmd, err := cc.expand(args, vars)
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	// Log the output

	t.Log(cmd.CommandName)
	t.Log(cmd.ShortName)
	t.Log(cmd.Args)

	for _, o := range cmd.ExpectedOutput {
		t.Log(o.Text)
	}

	// Assertions

	assert.Equal("/testbin/add", cmd.CommandName)
	assert.Equal("add", cmd.ShortName)

	assert.Equal(1, len(cmd.ExpectedOutput))

	if len(cmd.ExpectedOutput) == 1 {
		assert.Equal("270", cmd.ExpectedOutput[0].Text)
	}
}

func TestCommandInput(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	// Create the Command Template
	cc := &CommandTemplate{
		Name:   "/testbin/ranger",
		Output: []TemplOutputLine{TemplOutputLine{"SUCCESS", "true", "false"}},
		Input:  []string{input_template},
	}

	// Create the command instance
	args := []string{}
	vars := []string{}

	cmd, err := cc.expand(args, vars)
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	// Log the output

	t.Log(cmd.CommandName)
	t.Log(cmd.ShortName)
	t.Log(cmd.Args)

	for _, o := range cmd.ExpectedOutput {
		t.Log(o.Text)
	}

	// Assertions
	assert.True(len(cmd.Args) >= 2)
	assert.True(len(cmd.Args) <= 10)

	for _, o := range cmd.Args {
		assert.True(len(o) >= 5)
		assert.True(len(o) <= 10)
	}
}

func TestCommandTemplateLoad(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	text := `---
templates:
  - name: sy1
  - name: sy2
  - name: sy3
  - name: sy4
  - name: sy5
`
	cmds, err := CommandTemplatesFromString(text)
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	assert.Equal(5, len(cmds.Templates))
	if len(cmds.Templates) == 5 {
		assert.Equal("sy1", cmds.Templates[0].Name)
		assert.Equal("sy2", cmds.Templates[1].Name)
		assert.Equal("sy3", cmds.Templates[2].Name)
		assert.Equal("sy4", cmds.Templates[3].Name)
		assert.Equal("sy5", cmds.Templates[4].Name)
	}
}
