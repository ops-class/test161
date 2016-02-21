package main

import (
	"errors"
	"fmt"
	"github.com/ops-class/test161"
	"os"
	"os/signal"
)

// test161 Submission Server

// Run cleanup when signal is received
type test161Server interface {
	Start()
	Stop()
}

var servers = []test161Server{}

// Modified from http://nathanleclaire.com/blog/2014/08/24/handling-ctrl-c-interrupt-signal-in-golang-programs/
func waitForSignal() {
	signalChan := make(chan os.Signal, 1)
	doneChan := make(chan bool)
	signal.Notify(signalChan, os.Interrupt)
	signal.Notify(signalChan, os.Kill)

	go func() {
		for _ = range signalChan {
			for _, s := range servers {
				s.Stop()
			}
			fmt.Println("Killing...")
			doneChan <- true
		}
	}()

	<-doneChan
}

func main() {
	// TODO: Usage

	if len(os.Args) > 1 {
		var err error
		var status int

		switch os.Args[1] {
		case "status":
			status, err = CtrlStatus()
			if err == nil {
				if status == test161.SM_ACCEPTING {
					fmt.Println("test161 server: accepting submissions")
				} else {
					fmt.Println("test161 server: not accepting submissions")
				}
			}
		case "pause":
			err = CtrlPause()
		case "resume":
			err = CtrlResume()
		case "set-capacity":
			if len(os.Args) != 3 {
				err = errors.New("Wrong number of arguments to set-capacity")
			} else {
				err = CtrlSetCapacity(os.Args[2])
			}
		case "get-capacity":
			var capacity int
			capacity, err = CtrlGetCapacity()
			if err == nil {
				fmt.Println("Current test capacity:", capacity)
			}
		case "version":
			fmt.Printf("test161-server version: %v\n", test161.Version)
			err = nil

		default:
			fmt.Println("Unknown command:", os.Args[1])
			os.Exit(2)
		}

		if err != nil {
			fmt.Println("Error processing request:", err)
			os.Exit(1)
		} else {
			os.Exit(0)
		}
	}

	// Create Submission Server
	server, err := NewSubmissionServer()
	if err != nil {
		fmt.Println("Error creating submission server:", err)
		return
	}
	servers = append(servers, server)

	ctrl := &ControlServer{}
	servers = append(servers, ctrl)

	for _, s := range servers {
		go s.Start()
	}

	waitForSignal()
}
