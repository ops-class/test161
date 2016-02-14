package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/ops-class/test161"
	"github.com/parnurzeal/gorequest"
	"os"
	"strings"
)

var listRemoteFlag bool

func doListTargets() {
	if err := getListArgs(); err != nil {
		printRunError(err)
		return
	}

	var targets *test161.TargetList

	if listRemoteFlag {
		var errs []error
		if targets, errs = getRemoteTargets(); len(errs) > 0 {
			printRunErrors(errs)
			return
		}
	} else {
		targets = env.TargetList()
	}

	printTargets(targets)
}

func getRemoteTargets() (*test161.TargetList, []error) {
	if len(conf.Server) == 0 {
		return nil, []error{errors.New("server field missing in .test161.conf")}
	}

	endpoint := conf.Server
	if !strings.HasPrefix(endpoint, "/") {
		endpoint += "/"
	}
	endpoint += "targets"

	request := gorequest.New()

	fmt.Println("Contacting", endpoint)

	_, body, errs := request.Get(endpoint).End()
	if errs != nil {
		return nil, errs
	}

	targets := &test161.TargetList{}

	if err := json.Unmarshal([]byte(body), targets); err != nil {
		return nil, []error{err}
	}

	return targets, nil
}

func printTargets(list *test161.TargetList) {
	if listRemoteFlag {
		fmt.Println("\nRemote Targets")
	} else {
		fmt.Println("\nLocal Targets")
	}
	fmt.Println(strings.Repeat("-", 40))

	for _, t := range list.Targets {
		fmt.Printf("%-30v v%v\n", t.Name, t.Version)
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
