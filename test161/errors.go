package main

import (
	"errors"
	"fmt"
	"github.com/fatih/color"
	"os"
)

var errText = color.New(color.Bold).SprintFunc()("Error:")

func __printRunError(err error) {
	fmt.Fprintf(os.Stderr, "%v %v\n", errText, err)
}

func printRunError(err error) {
	__printRunError(err)
}

func printRunErrors(errs []error) {
	if len(errs) > 0 {
		for _, e := range errs {
			__printRunError(e)
		}
	}
}

func connectionError(endpoint string, errs []error) []error {
	msg := fmt.Sprintf("Unable to connect to server '%v'", endpoint)
	for _, e := range errs {
		msg += fmt.Sprintf("\n    %v", e)
	}
	return []error{errors.New(msg)}
}
