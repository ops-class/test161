package main

import (
	"fmt"
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

	// Create Submission Server
	server, err := NewSubmissionServer()
	if err != nil {
		fmt.Println("Error creating submission server:", err)
		return
	}
	servers = append(servers, server)

	// Eventually we'll add stats and control servers

	for _, s := range servers {
		go s.Start()
	}

	waitForSignal()
}
