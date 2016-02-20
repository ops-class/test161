package main

import (
	"errors"
	"log"
	"net"
	"net/http"
	"net/rpc"
)

const (
	CTRL_PAUSE = iota
	CTRL_RESUME
	CTRL_STATUS
)

type ControlRequest struct {
	Message int
}

type ServerCtrl int

func (sc *ServerCtrl) Control(msg *ControlRequest, reply *int) error {

	*reply = 0

	if submissionMgr == nil {
		return errors.New("SubmissionManager is not initialized")
	}

	switch msg.Message {
	case CTRL_PAUSE:
		submissionMgr.Pause()
		return nil
	case CTRL_RESUME:
		submissionMgr.Resume()
		return nil
	case CTRL_STATUS:
		*reply = submissionMgr.Status()
		return nil
	default:
		return errors.New("Unrecongnized control message")
	}
}

type ControlServer struct {
}

func (cs *ControlServer) Start() {
	server := rpc.NewServer()
	server.Register(new(ServerCtrl))
	server.HandleHTTP("/test161/control", "/debug/test161/control")
	l, e := net.Listen("tcp", "127.0.0.1:4001")
	if e != nil {
		log.Fatal("listen error:", e)
	}
	http.Serve(l, nil)
}

func (server *ControlServer) Stop() {
}

func doCtrlRequest(msg int, reply *int) error {
	client, err := rpc.DialHTTPPath("tcp", "127.0.0.1:4001", "/test161/control")
	if err != nil {
		return err
	}

	// Synchronous call
	req := ControlRequest{msg}
	err = client.Call("ServerCtrl.Control", req, reply)
	return err
}

func CtrlPause() error {
	var reply int
	return doCtrlRequest(CTRL_PAUSE, &reply)
}

func CtrlResume() error {
	var reply int
	return doCtrlRequest(CTRL_RESUME, &reply)
}

func CtrlStatus() (int, error) {
	var reply int
	err := doCtrlRequest(CTRL_STATUS, &reply)
	return reply, err
}
