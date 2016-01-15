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

var validStat = regexp.MustCompile(`^DATA\s+(?P<Nanos>\d+)\s+(?P<Kern>\d+)\s+(?P<User>\d+)\s+(?P<Idle>\d+)\s+(?P<Kinsns>\d+)\s+(?P<Uinsns>\d+)\s+(?P<IRQs>\d+)\s+(?P<Exns>\d+)\s+(?P<Disk>\d+)\s+(?P<Con>\d+)\s+(?P<Emu>\d+)\s+(?P<Net>\d+)`)

type Stat struct {
	initialized bool

	Start  TimeFixedPoint `json:"start"`
	End    TimeFixedPoint `json:"end"`
	Length TimeFixedPoint `json:"length"`
	Count  uint           `json:"count"`

	WallStart  TimeFixedPoint `json:"wallstart"`
	WallEnd    TimeFixedPoint `json:"wallend"`
	WallLength TimeFixedPoint `json:"walllength"`

	Nanos  uint64 `json:"-"`
	Cycles uint32 `json:"cycles"`
	Kern   uint32 `json:"kern"`
	User   uint32 `json:"user"`
	Idle   uint32 `json:"idle"`
	Kinsns uint32 `json:"kinsns"`
	Uinsns uint32 `json:"uinsns"`
	IRQs   uint32 `json:"irqs"`
	Exns   uint32 `json:"exns"`
	Disk   uint32 `json:"disk"`
	Con    uint32 `json:"con"`
	Emu    uint32 `json:"emu"`
	Net    uint32 `json:"net"`
}

// Add adds two stat objects.
func (i *Stat) Add(j Stat) {
	i.Cycles += j.Cycles
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
	i.Cycles -= j.Cycles
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

// Append appends the stat object to an existing stat object.
func (i *Stat) Append(j Stat) {
	if i.initialized == false {
		i.initialized = true
		i.Count = 0

		i.Start = j.Start
		i.WallStart = j.WallStart
	} else {
		if i.End-j.Start > 0.00001 {
			panic("test161: merge stats should be adjacent")
		}
	}
	i.Count += 1

	i.End = j.End
	i.Length = TimeFixedPoint(float64(i.End) - float64(i.Start))

	i.WallEnd = j.WallEnd
	i.WallLength = TimeFixedPoint(float64(i.WallEnd) - float64(i.WallStart))

	i.Add(j)
}

// Shift shifts the stat object from an existing stat object.
func (i *Stat) Shift(j Stat) {
	if i.initialized == false {
		panic("test161: can't unshift from an empty stat object")
	}
	if i.Start-j.Start > 0.00001 {
		panic("test161: unshift stats should be adjacent")
	}
	i.Count -= 1

	i.Start = j.End
	i.Length = TimeFixedPoint(float64(i.End) - float64(i.Start))

	i.WallStart = j.WallEnd
	i.WallLength = TimeFixedPoint(float64(i.WallEnd) - float64(i.WallStart))

	i.Sub(j)
}

// stopStats disables stats collection.
func (t *Test) stopStats(Status string, ShutdownMessage string) {

	// This is normal shutdown.
	if t.statError == io.EOF {
		t.statError = nil
		Status = ""
	}

	// Mark failure and kill sys161
	t.L.Lock()
	if Status != "" && t.Status != "" {
		t.Status = Status
		t.ShutdownMessage = ShutdownMessage
	}
	t.L.Unlock()
	t.sys161.Killer()
}

// getStats is the main stats collection and monitor goroutine.
func (t *Test) getStats() {
	defer close(t.statChan)

	// Connect to the sys161 stats socket.
	var statConn net.Conn
	statConn, t.statError = net.Dial("unix", path.Join(t.tempDir, ".sockets/meter"))
	if t.statError != nil {
		t.stopStats("stats", "couldn't connect")
	}

	// Configure stat interval.
	_, t.statError =
		statConn.Write([]byte(fmt.Sprintf("INTERVAL %v\n", uint32(t.Stat.Resolution*1000*1000*1000))))
	if t.statError != nil {
		t.stopStats("stats", "couldn't set interval")
	}

	// Set up previous stat values and timestamps for diffs.
	wallStart := t.getWallTime()
	simStart := TimeFixedPoint(float64(0.0))
	lastStat := Stat{}

	// Stats cache for the monitor. Not needed when monitoring is disabled.
	monitorWindow := &Stat{}
	var monitorCache []Stat
	if t.Monitor.Enabled == "true" {
		monitorCache = make([]Stat, 0, t.Monitor.Window)
	}

	// Record when to flush stat cache
	lastCounter := -1

	statReader := bufio.NewReader(statConn)
	for {
		var line string

		// Grab a stat message.
		line, t.statError = statReader.ReadString('\n')
		if t.statError != nil {
			// This error gets cleared if we've read EOF, since that's normal
			// shutdown.
			t.stopStats("stats", "problem reading stats")
			return
		}
		// Set the timestamp
		wallEnd := t.getWallTime()

		// Make sure it's a data message and not something else. We ignore other
		// messages.
		statMatch := validStat.FindStringSubmatch(line)
		if statMatch == nil {
			continue
		}

		// Pulse the CV to free the main loop if needed and update our recording
		// and monitoring flags
		t.statCond.L.Lock()
		statRecord := t.statRecord
		statMonitor := t.statMonitor
		t.statCond.Signal()
		t.statCond.L.Unlock()

		// Create the new stat object and update timestamps
		stats := Stat{
			WallStart:  wallStart,
			WallEnd:    wallEnd,
			WallLength: TimeFixedPoint(float64(wallEnd) - float64(wallStart)),
		}
		wallStart = wallEnd

		// A bit of reflection to move data from the regexp match to the
		// stat object...
		s := reflect.ValueOf(&stats).Elem()
		for i, name := range validStat.SubexpNames() {
			f := s.FieldByName(name)
			x, err := strconv.ParseUint(statMatch[i], 10, 32)
			if err != nil {
				continue
			}
			f.SetUint(x)
		}
		// ... which doesn't work for all fields
		stats.Nanos, _ = strconv.ParseUint(statMatch[1], 10, 64)
		stats.Cycles = stats.Kern + stats.User + stats.Idle

		// Parse the simulation timestamps and update our boundaries
		stats.Start = simStart
		stats.End = TimeFixedPoint(float64(stats.Nanos) / 1000000000.0)
		stats.Length = TimeFixedPoint(float64(stats.End) - float64(stats.Start))
		simStart = stats.End

		// sys161 stat objects are cumulative, but we want incremental.
		temp := stats
		stats.Sub(lastStat)
		lastStat = temp

		// Non-blocking send of new stats
		select {
		case t.statChan <- stats:
		default:
		}

		// Update shared state, caching some values to use after dropping the lock
		t.L.Lock()
		t.SimTime = stats.End
		if statRecord {
			if (len(t.currentCommand.AllStats) == 0) ||
				(t.currentCommand.AllStats[len(t.currentCommand.AllStats)-1].Count == t.Stat.Window) {
				t.currentCommand.AllStats = append(t.currentCommand.AllStats, Stat{})
			}
			t.currentCommand.AllStats[len(t.currentCommand.AllStats)-1].Append(stats)
			t.currentCommand.SummaryStats.Append(stats)
		}
		// Cached for use by the monitoring code below
		progressTime := float64(t.SimTime) - t.progressTime
		currentCounter := t.commandCounter
		currentType := t.currentCommand.Type
		t.L.Unlock()

		// If we've moved forward one command clear the stat cache.
		if statRecord && int(currentCounter) != lastCounter {
			monitorCache = nil
			monitorWindow = &Stat{}
			lastCounter = int(currentCounter)
		}

		// Monitoring code starts here. Only run the monitor if it's enabled
		// globally, for this command (statMonitor), and at this time
		// (statRecord).
		if t.Monitor.Enabled != "true" || !statMonitor || !statRecord {
			continue
		}

		// Update the statCache and moving window.
		if uint(len(monitorCache)) == t.Monitor.Window {
			var head Stat
			head, monitorCache = monitorCache[0], monitorCache[1:]
			monitorWindow.Shift(head)
		}
		monitorCache = append(monitorCache, stats)
		monitorWindow.Append(stats)

		// Begin checks for various error conditions. No real rhyme or reason to
		// the order here. We could return multiple errors but that would be a bit
		// of a pain.
		monitorError := ""
		if progressTime > float64(t.Monitor.ProgressTimeout) {
			monitorError =
				fmt.Sprintf("no progress for %v s in %v mode", t.Monitor.ProgressTimeout, currentType)
		}
		// Only run these checks if we have enough state
		if uint(len(monitorCache)) >= t.Monitor.Window {
			if currentType == "kernel" && monitorWindow.User > 0 {
				monitorError = "non-zero user cycles during kernel operation"
			} else if t.Monitor.Kernel.EnableMin == "true" &&
				float64(monitorWindow.Kern)/float64(monitorWindow.Cycles) < t.Monitor.Kernel.Min {
				monitorError = "insufficient kernel cycles (potential deadlock)"
			} else if float64(monitorWindow.Kern)/float64(monitorWindow.Cycles) > t.Monitor.Kernel.Max {
				monitorError = "too many kernel cycles (potential livelock)"
			} else if currentType == "user" && t.Monitor.User.EnableMin == "true" &&
				(float64(monitorWindow.User)/float64(monitorWindow.Cycles) < t.Monitor.User.Min) {
				monitorError = "insufficient user cycles"
			} else if currentType == "user" &&
				(float64(monitorWindow.User)/float64(monitorWindow.Cycles) > t.Monitor.User.Max) {
				monitorError = "too many user cycles"
			}
		}

		// Before we blow up check to make sure that we haven't moved on to a
		// different command or disabled recording or monitoring while we've been
		// calculating.
		if monitorError != "" {
			t.statCond.L.Lock()
			blowup := (t.statRecord && t.statMonitor)
			t.statCond.L.Unlock()
			if blowup {
				t.L.Lock()
				blowup = blowup && (currentCounter == t.commandCounter)
				t.L.Unlock()
				if blowup {
					t.stopStats("monitor", monitorError)
				}
			}
		}
	}
}

// enableStats turns on stats collection
func (t *Test) enableStats() error {
	t.statCond.L.Lock()
	defer t.statCond.L.Unlock()
	t.statRecord = true
	t.statMonitor = t.currentCommand.Monitored
	t.statCond.Wait()
	return t.statError
}

// enableStats disables stats collection
func (t *Test) disableStats() error {
	t.statCond.L.Lock()
	defer t.statCond.L.Unlock()
	t.statRecord = false
	return t.statError
}
