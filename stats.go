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

// Add adds two stat objects.
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

// Sub subtracts two stat objects.
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

// Appends the stat object to an existing stat object.
func (i *Stat) Append(j Stat) {
	if i.initialized == false {
		i.Start = j.Start
		i.WallStart = j.WallStart
		i.initialized = true
	} else {
		if i.End-j.Start > 0.00001 {
			panic("test161: merged stats should be adjacent")
		}
	}
	i.End = j.End
	i.WallEnd = j.WallEnd
	i.Length = TimeFixedPoint(float64(i.End) - float64(i.Start))
	i.WallLength = TimeFixedPoint(float64(i.WallEnd) - float64(i.WallStart))
	i.Add(j)
}

// Shifts the stat object from an existing stat object.
func (i *Stat) Shift(j Stat) {
	if i.initialized == false {
		panic("test161: can't unshift from an empty stat object")
	}
	if i.Start-j.Start > 0.00001 {
		panic("test161: unshifted stats should be adjacent")
	}
	i.Start = j.End
	i.WallStart = j.WallEnd
	i.Length = TimeFixedPoint(float64(i.End) - float64(i.Start))
	i.WallLength = TimeFixedPoint(float64(i.WallEnd) - float64(i.WallStart))
	i.Sub(j)
}

// stopStats disables stats collection
func (t *Test) stopStats() {
	// Pulse the CV to free the main loop.
	t.statCond.L.Lock()
	if t.statError == io.EOF {
		// This is normal shutdown
		t.statError = nil
	}
	t.statActive = false
	t.statCond.Signal()
	t.statCond.L.Unlock()
}

// getStats is the main stats collection and monitor goRoutine. Note that we
// don't need to worry about shutdown much because ReadString will return EOF
// once the expect process is terminated.
func (t *Test) getStats() {
	defer close(t.statChan)

	// Connect to the sys161 stats socket, with deferred cleanup
	var statConn net.Conn
	statConn, t.statError = net.Dial("unix", path.Join(t.tempDir, ".sockets/meter"))
	if t.statError != nil {
		t.commandLock.Lock()
		t.Status = "monitor"
		t.ShutdownMessage = "couldn't connect"
		// Kill sys161 process
		t.sys161.Killer()
		t.commandLock.Unlock()
		return
	}
	defer t.stopStats()
	statReader := bufio.NewReader(statConn)

	// Set up previous stat values and timestamps for diffs
	wallStart := t.getWallTime()
	simStart := TimeFixedPoint(float64(0.0))
	lastStat := Stat{}

	// Statistics cache for the monitor. Not needed when monitoring is disabled.
	var intervals uint
	var statCache []Stat
	intervalStat := &Stat{}
	if t.MonitorConf.Enabled == "true" {
		intervals = uint(t.MonitorConf.Window * 1000.0 * 1000.0 / float32(t.MonitorConf.Resolution))
		statCache = make([]Stat, 0, intervals)
	}

	// Record when to flush stat cache
	lastCommandID := -1

	for {
		var line string

		// Grab a stat message
		line, t.statError = statReader.ReadString('\n')
		if t.statError != nil {
			// stopStats will free the main loop
			return
		}

		// Make sure it's a data message and not something else. We ignore other
		// messages.
		statMatch := validStat.FindStringSubmatch(line)
		if statMatch == nil {
			continue
		}

		// Set the timestamp
		wallEnd := t.getWallTime()

		// Pulse the CV to free the main loop if needed and update our recording
		// and monitoring flags
		t.statCond.L.Lock()
		t.statActive = true
		statRecord := t.statRecord
		statMonitor := t.statMonitor
		t.statCond.Signal()
		t.statCond.L.Unlock()

		// Create the new stat object and update timestamps
		newStats := Stat{
			WallStart:  wallStart,
			WallEnd:    wallEnd,
			WallLength: TimeFixedPoint(float64(wallEnd) - float64(wallStart)),
		}
		wallStart = wallEnd

		// A bit of reflection to move data from the regexp match to the
		// stat object...
		s := reflect.ValueOf(&newStats).Elem()
		for i, name := range validStat.SubexpNames() {
			f := s.FieldByName(name)
			x, err := strconv.ParseUint(statMatch[i], 10, 32)
			if err != nil {
				continue
			}
			f.SetUint(x)
		}
		// ...but it doesn't work for this one 64-bit field. Ugh.
		newStats.Nanos, _ = strconv.ParseUint(statMatch[len(statMatch)-1], 10, 64)

		// Parse the simulation timestamps and update our boundaries
		newStats.Start = simStart
		newStats.End = TimeFixedPoint(float64(newStats.Nanos) / 1000000000.0)
		newStats.Length = TimeFixedPoint(float64(newStats.End) - float64(newStats.Start))
		simStart = newStats.End

		// Non-blocking send of new stats
		select {
		case t.statChan <- newStats:
		default:
		}

		// sys161 stat objects are cumulative, but we want incremental.
		tempStat := newStats
		newStats.Sub(lastStat)
		lastStat = tempStat

		// Update shared state, caching some values to use after dropping the lock
		t.commandLock.Lock()
		t.SimTime = simStart
		if statRecord {
			if t.MonitorConf.AllStats == "true" {
				t.command.AllStats = append(t.command.AllStats, newStats)
			}
			t.command.SummaryStats.Append(newStats)
		}
		// Cached for use by the monitoring code below
		progressTime := float64(t.SimTime) - t.progressTime
		currentCommandID := t.command.ID
		currentEnv := t.command.Env
		t.commandLock.Unlock()

		// If we've moved forward one command clear the stat cache.
		if statRecord && int(currentCommandID) != lastCommandID {
			statCache = nil
			lastCommandID = int(currentCommandID)
		}

		// Monitoring code starts here. Only run the monitor if it's enabled
		// globally, for this command (statMonitor), and at this time
		// (statRecord).
		if t.MonitorConf.Enabled != "true" || !statMonitor || !statRecord {
			continue
		}

		// Update the statCache and moving window. Probably more efficient ways to
		// do this but this seems OK for now.
		if uint(len(statCache)) == intervals {
			var head Stat
			head, statCache = statCache[0], statCache[1:]
			intervalStat.Shift(head)
		}
		statCache = append(statCache, newStats)
		intervalStat.Append(newStats)
		if uint(len(statCache)) < intervals {
			continue
		}

		// Begin checks for various error conditions. No real rhyme or reason to
		// the order here. We could return multiple errors but that would be a bit
		// of a pain.
		monitorError := ""

		if progressTime > float64(t.MonitorConf.Timeouts.Progress) {
			monitorError = fmt.Sprintf("no progress for %d s", t.MonitorConf.Timeouts.Progress)
		} else if currentEnv == "kernel" && intervalStat.User > 0 {
			monitorError = "non-zero user cycles during kernel operation"
		} else if float64(intervalStat.Kern)/
			float64(intervalStat.Kern+intervalStat.User+intervalStat.Idle) < t.MonitorConf.Kernel.Min {
			monitorError = "insufficient kernel cycle (potential deadlock)"
		} else if float64(intervalStat.Kern)/
			float64(intervalStat.Kern+intervalStat.User+intervalStat.Idle) > t.MonitorConf.Kernel.Max {
			monitorError = "too many kernel cycle (potential livelock)"
		} else if currentEnv == "shell" && (float64(intervalStat.User)/
			float64(intervalStat.Kern+intervalStat.User+intervalStat.Idle) < t.MonitorConf.User.Min) {
			monitorError = "insufficient user cycles"
		} else if currentEnv == "shell" && (float64(intervalStat.User)/
			float64(intervalStat.Kern+intervalStat.User+intervalStat.Idle) > t.MonitorConf.User.Max) {
			monitorError = "too many user cycles"
		}

		// Before we blow up check to make sure that we haven't moved on to a
		// different command or disabled recording or monitoring while we've been
		// calculating.
		if monitorError != "" {
			t.statCond.L.Lock()
			statRecord = t.statRecord
			statMonitor = t.statMonitor
			t.statCond.L.Unlock()

			t.commandLock.Lock()
			if currentCommandID == t.command.ID && statRecord && statMonitor {
				// Blow up.
				t.Status = "monitor"
				t.ShutdownMessage = monitorError
				t.sys161.Killer()
			}
			t.commandLock.Unlock()
		}
	}
}
