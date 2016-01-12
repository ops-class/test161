package test161

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"path"
	"reflect"
	"regexp"
	"strconv"
)

var validStat = regexp.MustCompile(`^DATA\s+(?P<Kern>\d+)\s+(?P<User>\d+)\s+(?P<Idle>\d+)\s+(?P<Kinsns>\d+)\s+(?P<Uinsns>\d+)\s+(?P<IRQs>\d+)\s+(?P<Exns>\d+)\s+(?P<Disk>\d+)\s+(?P<Con>\d+)\s+(?P<Emu>\d+)\s+(?P<Net>\d+)\s+(?P<Nanos>\d+)`)

type Stat struct {
	initialized bool
	Start       TimeFixedPoint `json:"start"`
	End         TimeFixedPoint `json:"end"`
	Length      TimeFixedPoint `json:"length"`
	WallStart   TimeFixedPoint `json:"wallstart"`
	WallEnd     TimeFixedPoint `json:"wallend"`
	WallLength  TimeFixedPoint `json:"walllength"`
	Kern        uint32         `json:"kern"`
	User        uint32         `json:"user"`
	Idle        uint32         `json:"idle"`
	Kinsns      uint32         `json:"kinsns"`
	Uinsns      uint32         `json:"uinsns"`
	IRQs        uint32         `json:"irqs"`
	Exns        uint32         `json:"exns"`
	Disk        uint32         `json:"disk"`
	Con         uint32         `json:"con"`
	Emu         uint32         `json:"emu"`
	Net         uint32         `json:"net"`
	Nanos       uint64         `json:"-"`
}

func (i *Stat) Add(j Stat) {
	i.Kern += j.Kern
	i.User += j.User
	i.Idle += j.Idle
	i.Kinsns += j.Kinsns
	i.Uinsns += j.Uinsns
	i.IRQs += j.IRQs
	i.Exns += j.Exns
	i.Disk += j.Disk
	i.Con += j.Con
	i.Emu += j.Emu
	i.Net += j.Net
	i.Nanos += j.Nanos
}
func (i *Stat) Sub(j Stat) {
	i.Kern -= j.Kern
	i.User -= j.User
	i.Idle -= j.Idle
	i.Kinsns -= j.Kinsns
	i.Uinsns -= j.Uinsns
	i.IRQs -= j.IRQs
	i.Exns -= j.Exns
	i.Disk -= j.Disk
	i.Con -= j.Con
	i.Emu -= j.Emu
	i.Net -= j.Net
}
func (i *Stat) Merge(j Stat) {
	if i.initialized == false {
		i.Start = j.Start
		i.WallStart = j.WallStart
		i.initialized = true
	}
	i.End = j.End
	i.WallEnd = j.WallEnd
	i.Length = TimeFixedPoint(float64(i.End) - float64(i.Start))
	i.WallLength = TimeFixedPoint(float64(i.WallEnd) - float64(i.WallStart))
	i.Add(j)
}

func (t *Test) stopStats() {
	t.statCond.L.Lock()
	t.statActive = false
	t.statCond.Signal()
	t.statCond.L.Unlock()
}

func (t *Test) getStats() {
	var err error

	defer close(t.statChan)

	statConn, err := net.Dial("unix", path.Join(t.tempDir, ".sockets/meter"))
	if err != nil {
		t.commandLock.Lock()
		t.Status = "monitor"
		t.ShutdownMessage = "couldn't connect"
		t.sys161.Killer()
		t.commandLock.Unlock()
		return
	}
	defer t.stopStats()

	statReader := bufio.NewReader(statConn)

	var line string

	wallStart := t.getWallTime()
	start := TimeFixedPoint(float64(0.0))
	lastStat := Stat{}

	intervals := uint(t.MonitorConf.Window*1000.0*1000.0/float32(t.MonitorConf.Resolution)) + 1
	statCache := make([]Stat, 0, intervals)
	lastCommandID := -1

	for {
		if err == nil {
			line, err = statReader.ReadString('\n')
		}
		wallEnd := t.getWallTime()

		t.statCond.L.Lock()
		t.statActive = true
		recordStats := t.recordStats
		monitorStats := t.monitorStats
		if err != nil && err != io.EOF {
			t.statError = err
		}
		t.statCond.Signal()
		t.statCond.L.Unlock()

		if err != nil {
			return
		}

		statMatch := validStat.FindStringSubmatch(line)
		if statMatch == nil {
			continue
		}

		newStats := Stat{
			WallStart:  wallStart,
			WallEnd:    wallEnd,
			WallLength: TimeFixedPoint(float64(wallEnd) - float64(wallStart)),
		}

		wallStart = wallEnd
		s := reflect.ValueOf(&newStats).Elem()
		for i, name := range validStat.SubexpNames() {
			f := s.FieldByName(name)
			x, err := strconv.ParseUint(statMatch[i], 10, 32)
			if err != nil {
				continue
			}
			f.SetUint(x)
		}
		newStats.Nanos, _ = strconv.ParseUint(statMatch[len(statMatch)-1], 10, 64)

		newStats.Start = start
		newStats.End = TimeFixedPoint(float64(newStats.Nanos) / 1000000000.0)
		newStats.Length = TimeFixedPoint(float64(newStats.End) - float64(newStats.Start))
		start = newStats.End

		select {
		case t.statChan <- newStats:
		default:
		}

		tempStat := newStats
		newStats.Sub(lastStat)
		lastStat = tempStat

		t.commandLock.Lock()
		t.SimTime = start
		progressTime := float64(t.SimTime) - t.progressTime
		if recordStats {
			if int(t.command.ID) != lastCommandID {
				statCache = nil
				lastCommandID = int(t.command.ID)
			}
			if t.AllStats == "true" {
				t.command.AllStats = append(t.command.AllStats, newStats)
			}
			t.command.SummaryStats.Merge(newStats)
		}
		currentCommandID := t.command.ID
		currentEnv := t.command.Env
		t.commandLock.Unlock()

		if t.MonitorConf.Enabled != "true" || !monitorStats || !recordStats {
			continue
		}
		if uint(len(statCache)) == intervals {
			statCache = statCache[1:]
		}
		statCache = append(statCache, newStats)
		if uint(len(statCache)) < intervals {
			continue
		}
		intervalStat := &Stat{}
		for _, stat := range statCache {
			intervalStat.Merge(stat)
		}

		monitorError := ""

		if progressTime > float64(t.MonitorConf.Timeouts.Progress) {
			monitorError = fmt.Sprintf("no progress for %d s", t.MonitorConf.Timeouts.Progress)
		}

		if currentEnv == "kernel" && intervalStat.User > 0 {
			monitorError = "non-zero user cycles during kernel operation"
		}

		if float64(intervalStat.Kern)/
			float64(intervalStat.Kern+intervalStat.User+intervalStat.Idle) < t.MonitorConf.Kernel.Min {
			monitorError = "insufficient kernel cycle (potential deadlock)"
		}
		if float64(intervalStat.Kern)/
			float64(intervalStat.Kern+intervalStat.User+intervalStat.Idle) > t.MonitorConf.Kernel.Max {
			monitorError = "too many kernel cycle (potential livelock)"
		}

		if currentEnv == "shell" && (float64(intervalStat.User)/
			float64(intervalStat.Kern+intervalStat.User+intervalStat.Idle) < t.MonitorConf.User.Min) {
			monitorError = "insufficient user cycles"
		}
		if currentEnv == "shell" && (float64(intervalStat.User)/
			float64(intervalStat.Kern+intervalStat.User+intervalStat.Idle) > t.MonitorConf.User.Max) {
			monitorError = "too many user cycles"
		}

		t.statCond.L.Lock()
		recordStats = t.recordStats
		monitorStats = t.monitorStats
		t.statCond.L.Unlock()

		if monitorError != "" {
			t.commandLock.Lock()
			if currentCommandID == t.command.ID && recordStats && monitorStats {
				t.Status = "monitor"
				t.ShutdownMessage = monitorError
				t.sys161.Killer()
			}
			t.commandLock.Unlock()
		}
	}
}
