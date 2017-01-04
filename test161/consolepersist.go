package main

import (
	"fmt"
	"github.com/ops-class/test161"
	"strings"
)

// A PersistenceManager that "persists" to the console

type ConsolePersistence struct {
	width int
}

func (c *ConsolePersistence) Close() {
}

const lineFmt = "[%-*v]\t%.6f\t%s"

func (c *ConsolePersistence) Notify(entity interface{}, msg, what int) error {

	if msg == test161.MSG_PERSIST_UPDATE && what == test161.MSG_FIELD_OUTPUT {

		switch entity.(type) {
		case *test161.Command:
			{
				cmd := entity.(*test161.Command)
				line := cmd.Output[len(cmd.Output)-1]
				// Learn the width on the fly (submissions)
				if c.width < len(cmd.Test.DependencyID) {
					c.width = len(cmd.Test.DependencyID)
				}
				output := fmt.Sprintf(lineFmt, c.width, cmd.Test.DependencyID, line.SimTime, line.Line)
				fmt.Println(output)
			}
		case *test161.BuildCommand:
			{
				cmd := entity.(*test161.BuildCommand)
				for _, line := range cmd.Output {
					output := fmt.Sprintf("%.6f\t%s", line.SimTime, line.Line)
					fmt.Println(output)
				}
			}
		}
	} else if msg == test161.MSG_PERSIST_UPDATE && what == test161.MSG_FIELD_STATUSES {
		switch entity.(type) {
		case *test161.Test:
			{
				test := entity.(*test161.Test)
				index := len(test.Status) - 1
				if index > 0 {
					status := test.Status[index]
					str := fmt.Sprintf(lineFmt, c.width, test.DependencyID, test.SimTime, status.Status)
					if status.Message != "" {
						str += fmt.Sprintf(": %s", status.Message)
					}
					fmt.Println(str)
				}
			}
		}
	} else if msg == test161.MSG_PERSIST_UPDATE && what == test161.MSG_FIELD_STATUS {
		switch entity.(type) {
		case *test161.Test:
			{
				test := entity.(*test161.Test)
				if test.Result == test161.TEST_RESULT_RUNNING {
					lines := strings.Split(strings.TrimSpace(test.ConfString), "\n")
					for _, line := range lines {
						output := fmt.Sprintf(lineFmt, c.width, test.DependencyID, 0.0, line)
						fmt.Println(output)
					}
				}
			}
		}
	}

	return nil
}

func (d *ConsolePersistence) Retrieve(what int, who map[string]interface{}, res interface{}) error {
	return nil
}

func (d *ConsolePersistence) CanRetrieve() bool {
	return false
}
