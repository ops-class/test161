package test161

import (
	"bufio"
	"io"
	"net"
	"path"
	"reflect"
	"regexp"
	"strconv"
)

var validStat = regexp.MustCompile(`^DATA\s+(?P<Kern>\d+)\s+(?P<User>\d+)\s+(?P<Idle>\d+)\s+(?P<Kinsns>\d+)\s+(?P<Uinsns>\d+)\s+(?P<IRQs>\d+)\s+(?P<Exns>\d+)\s+(?P<Disk>\d+)\s+(?P<Con>\d+)\s+(?P<Emu>\d+)\s+(?P<Net>\d+)`)

type Stat struct {
	initialized bool
	Start       TimeDelta `json:"start"`
	End         TimeDelta `json:"end"`
	Length      TimeDelta `json:"length"`
	Kern        uint32    `json:"kern"`
	User        uint32    `json:"user"`
	Idle        uint32    `json:"idle"`
	Kinsns      uint32    `json:"kinsns"`
	Uinsns      uint32    `json:"uinsns"`
	IRQs        uint32    `json:"irqs"`
	Exns        uint32    `json:"exns"`
	Disk        uint32    `json:"disk"`
	Con         uint32    `json:"con"`
	Emu         uint32    `json:"emu"`
	Net         uint32    `json:"net"`
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
		i.initialized = true
	}
	i.End = j.End
	i.Length = TimeDelta(float64(i.End) - float64(i.Start))
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

	start := t.getDelta()
	lastStat := Stat{}

	statCache := make([]Stat, 0, t.MonitorConf.Intervals)

	for {
		if err == nil {
			line, err = statReader.ReadString('\n')
		}
		end := t.getDelta()

		t.statCond.L.Lock()
		t.statActive = true
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
			Start:  start,
			End:    end,
			Length: TimeDelta(float64(end) - float64(start)),
		}

		start = end
		s := reflect.ValueOf(&newStats).Elem()
		for i, name := range validStat.SubexpNames() {
			f := s.FieldByName(name)
			x, err := strconv.ParseUint(statMatch[i], 10, 32)
			if err != nil {
				continue
			}
			f.SetUint(x)
		}

		tempStat := newStats
		newStats.Sub(lastStat)
		lastStat = tempStat

		t.commandLock.Lock()
		if len(t.command.AllStats) == 0 {
			statCache = make([]Stat, 0, t.MonitorConf.Intervals)
		}
		t.command.AllStats = append(t.command.AllStats, newStats)
		t.command.SummaryStats.Merge(newStats)
		currentCommandID := t.command.ID
		currentEnv := t.command.Env
		t.commandLock.Unlock()

		if t.MonitorConf.Enabled != "true" {
			continue
		}
		if uint(len(statCache)) == t.MonitorConf.Intervals {
			statCache = statCache[1:]
		}
		statCache = append(statCache, newStats)
		if uint(len(statCache)) < t.MonitorConf.Intervals {
			continue
		}
		intervalStat := &Stat{}
		for _, stat := range statCache {
			intervalStat.Merge(stat)
		}

		monitorError := ""

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

		if monitorError != "" {
			t.commandLock.Lock()
			if currentCommandID == t.command.ID {
				t.Status = "monitor"
				t.ShutdownMessage = monitorError
				t.sys161.Killer()
			}
			t.commandLock.Unlock()
		}
	}
}
