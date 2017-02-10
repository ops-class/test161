package main

import (
	"errors"
	"github.com/ops-class/test161"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"strconv"
)

const (
	CTRL_PAUSE = iota
	CTRL_RESUME
	CTRL_STATUS
	CTRL_SETCAPACITY
	CTRL_GETCAPACITY
	CTRL_STAFF_ONLY
)

type ControlRequest struct {
	Message     int
	NewCapacity uint
}

type ServerCtrl int

func (sc *ServerCtrl) Control(msg *ControlRequest, reply *int) error {

	*reply = 0

	if submissionServer == nil || submissionServer.submissionMgr == nil {
		return errors.New("SubmissionManager is not initialized")
	}

	submissionMgr := submissionServer.submissionMgr

	switch msg.Message {
	case CTRL_PAUSE:
		submissionMgr.Pause()
		return nil
	case CTRL_RESUME:
		submissionMgr.Resume()
		return nil
	case CTRL_STAFF_ONLY:
		submissionMgr.SetStaffOnly()
		return nil
	case CTRL_STATUS:
		*reply = submissionMgr.Status()
		return nil
	case CTRL_GETCAPACITY:
		cap := test161.ManagerCapacity()
		*reply = int(cap)
		return nil
	case CTRL_SETCAPACITY:
		test161.SetManagerCapacity(msg.NewCapacity)
		*reply = 0
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

func doCtrlRequest(msg interface{}, reply *int) error {

	req := ControlRequest{}

	switch msg.(type) {
	case int:
		req.Message = msg.(int)
	case ControlRequest:
		req = msg.(ControlRequest)
	default:
		return errors.New("Unexpected type in doCtrlRequest")
	}

	client, err := rpc.DialHTTPPath("tcp", "127.0.0.1:4001", "/test161/control")
	if err != nil {
		return err
	}

	// Synchronous call
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

func CtrlSetStaffOnly() error {
	var reply int
	return doCtrlRequest(CTRL_STAFF_ONLY, &reply)
}

func CtrlStatus() (int, error) {
	var reply int
	err := doCtrlRequest(CTRL_STATUS, &reply)
	return reply, err
}

func CtrlGetCapacity() (int, error) {
	var reply int
	err := doCtrlRequest(CTRL_GETCAPACITY, &reply)
	return reply, err
}

func CtrlSetCapacity(argCap string) error {
	newCap, err := strconv.Atoi(argCap)
	if err != nil {
		return err
	}

	var reply int
	err = doCtrlRequest(ControlRequest{
		Message:     CTRL_SETCAPACITY,
		NewCapacity: uint(newCap),
	}, &reply)

	return err
}
