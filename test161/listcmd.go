package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/ops-class/test161"
	"github.com/parnurzeal/gorequest"
	"net/http"
	"os"
	"strings"
)

var listRemoteFlag bool

func doListTargets() int {
	if err := getListArgs(); err != nil {
		printRunError(err)
		return 1
	}

	var targets *test161.TargetList

	if listRemoteFlag {
		var errs []error
		if targets, errs = getRemoteTargets(); len(errs) > 0 {
			printRunErrors(errs)
			return 1
		}
	} else {
		targets = env.TargetList()
	}

	printTargets(targets)

	return 0
}

func getRemoteTargets() (*test161.TargetList, []error) {
	if len(conf.Server) == 0 {
		return nil, []error{errors.New("server field missing in .test161.conf")}
	}

	endpoint := conf.Server + "/api-v1/targets"
	request := gorequest.New()

	resp, body, errs := request.Get(endpoint).End()
	if errs != nil {
		return nil, errs
	}

	if resp.StatusCode != http.StatusOK {
		return nil, []error{fmt.Errorf("Unable to retrieve remote targets: %v", resp.Status)}
	}

	targets := &test161.TargetList{}

	if err := json.Unmarshal([]byte(body), targets); err != nil {
		return nil, []error{err}
	}

	return targets, nil
}

func printTargets(list *test161.TargetList) {
	var desc string
	if listRemoteFlag {
		desc = "Remote Target"
	} else {
		desc = "Local Target"
	}
	fmt.Printf("\n%-20v   %-6v   %-8v   %-7v\n", desc, "Type", "Version", "Points")

	fmt.Printf("%v   %v   %v   %v\n", strings.Repeat("-", 20),
		strings.Repeat("-", 6), strings.Repeat("-", 8), strings.Repeat("-", 7))

	for _, t := range list.Targets {
		fmt.Printf("%-20v   %-6v   v%-7v   %-7v\n", t.Name, t.Type, t.Version, t.Points)
	}

	fmt.Println()
}

func getListArgs() error {

	listFlags := flag.NewFlagSet("test161 list-targets", flag.ExitOnError)
	listFlags.Usage = usage
	listFlags.BoolVar(&listRemoteFlag, "remote", false, "")
	listFlags.BoolVar(&listRemoteFlag, "r", false, "")

	listFlags.Parse(os.Args[2:]) // this may exit

	if len(listFlags.Args()) > 0 {
		return errors.New("test161 list-targets does not support positional arguments")
	}

	return nil
}
