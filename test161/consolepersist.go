package main

import (
	"fmt"
	"github.com/ops-class/test161"
)

// A PersistenceManager that "persists" to the console

type ConsolePersistence struct {
}

func (c *ConsolePersistence) Close() {
}

func (c *ConsolePersistence) Notify(entity interface{}, msg, what int) error {

	if msg == test161.MSG_PERSIST_UPDATE && what == test161.MSG_FIELD_OUTPUT {

		switch entity.(type) {
		case *test161.Command:
			{
				cmd := entity.(*test161.Command)
				line := cmd.Output[len(cmd.Output)-1]
				output := fmt.Sprintf("%.6f\t%s", line.SimTime, line.Line)
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
					str := fmt.Sprintf("%.6f\t%s", test.SimTime, status.Status)
					if status.Message != "" {
						str += fmt.Sprintf(": %s", status.Message)
					}
					fmt.Println(str)
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
