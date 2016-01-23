package test161

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"path"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// sys161 2.0.5: nsec kinsns uinsns udud idle irqs exns disk con emu net
var validHead = regexp.MustCompile(`^HEAD\s+nsec\s+kinsns\s+uinsns\s+udud\s+idle\s+irqs\s+exns\s+disk\s+con\s+emu\s+net`)
var validStat = regexp.MustCompile(`^DATA\s+(?P<Nsec>\d+)\s+(?P<Kinsns>\d+)\s+(?P<Uinsns>\d+)\s+(?P<Udud>\d+)\s+(?P<Idle>\d+)\s+(?P<IRQs>\d+)\s+(?P<Exns>\d+)\s+(?P<Disk>\d+)\s+(?P<Con>\d+)\s+(?P<Emu>\d+)\s+(?P<Net>\d+)`)

type Stat struct {
	initialized bool

	Start  TimeFixedPoint `json:"start"`
	End    TimeFixedPoint `json:"end"`
	Length TimeFixedPoint `json:"length"`
	Count  uint           `json:"count"`

	WallStart  TimeFixedPoint `json:"wallstart"`
	WallEnd    TimeFixedPoint `json:"wallend"`
	WallLength TimeFixedPoint `json:"walllength"`

	// Read from stat line
	Nsec   uint64 `json:"-"`
	Kinsns uint32 `json:"kinsns"`
	Uinsns uint32 `json:"uinsns"`
	Udud   uint32 `json:"udud"`
	Idle   uint32 `json:"idle"`
	IRQs   uint32 `json:"irqs"`
	Exns   uint32 `json:"exns"`
	Disk   uint32 `json:"disk"`
	Con    uint32 `json:"con"`
	Emu    uint32 `json:"emu"`
	Net    uint32 `json:"net"`

	// Derived
	Insns uint32 `json:"insns"`
}

// Add adds two stat objects.
func (i *Stat) Add(j Stat) {
	i.Nsec += j.Nsec
	i.Kinsns += j.Kinsns
	i.Uinsns += j.Uinsns
	i.Udud += j.Udud
	i.Idle += j.Idle
	i.IRQs += j.IRQs
	i.Exns += j.Exns
	i.Disk += j.Disk
	i.Con += j.Con
	i.Emu += j.Emu
	i.Net += j.Net

	i.Insns += j.Insns
}

// Sub subtracts two stat objects.
func (i *Stat) Sub(j Stat) {
	i.Nsec -= j.Nsec
	i.Kinsns -= j.Kinsns
	i.Uinsns -= j.Uinsns
	i.Udud -= j.Udud
	i.Idle -= j.Idle
	i.IRQs -= j.IRQs
	i.Exns -= j.Exns
	i.Disk -= j.Disk
	i.Con -= j.Con
	i.Emu -= j.Emu
	i.Net -= j.Net

	i.Insns -= j.Insns
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
func (t *Test) stopStats(status string, message string, statErr error) {
	if status != "" {
		t.addStatus(status, message)
	}
	t.statCond.L.Lock()
	t.statErr = statErr
	t.statActive = false
	t.statCond.Signal()
	t.statCond.L.Unlock()
	t.stop161()
}

// getStats is the main stats collection and monitor goroutine.
func (t *Test) getStats() {
	defer close(t.statChan)

	// Mark started
	t.statCond.L.Lock()
	t.statActive = true
	t.statCond.L.Unlock()

	// Connect to the sys161 stats socket.
	var statConn net.Conn
	statConn, err := net.Dial("unix", path.Join(t.tempDir, ".sockets/meter"))
	if err != nil {
		t.stopStats("stats", "couldn't connect", err)
		return
	}

	// Configure stat interval.
	_, err =
		statConn.Write([]byte(fmt.Sprintf("INTERVAL %v\n", uint32(t.Stat.Resolution*1000*1000*1000))))
	if err != nil {
		t.stopStats("stats", "couldn't set interval", err)
		return
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
		line, err := statReader.ReadString('\n')
		if err == io.EOF {
			t.stopStats("", "", nil)
			return
		} else if err != nil {
			t.stopStats("stats", "problem reading stats", err)
			return
		}
		// Set the timestamp
		wallEnd := t.getWallTime()

		// Check HEAD messages
		if strings.HasPrefix(line, "HEAD ") && validHead.FindString(line) == "" {
			t.stopStats("stats", fmt.Sprintf("incorrect stat format: %v", line), errors.New("incorrect stat format"))
			return
		}

		// Ignore non-data messages
		if !strings.HasPrefix(line, "DATA ") {
			continue
		}

		// Make sure it's a data message and blow up if we can't parse it.
		statMatch := validStat.FindStringSubmatch(line)
		if statMatch == nil {
			t.stopStats("stats", "couldn't parse stat message", errors.New("couldn't parse stat message"))
			return
		}

		// Pulse the CV to free the main loop if needed and update our recording
		// and monitoring flags
		t.statCond.L.Lock()
		statRecord := t.statRecord
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
		stats.Nsec, _ = strconv.ParseUint(statMatch[1], 10, 64)
		// sys161 instructions are single-cycle, so we can combine idle (cycles)
		// with instructions
		stats.Insns = stats.Kinsns + stats.Uinsns + stats.Idle

		// Parse the simulation timestamps and update our boundaries
		stats.Start = simStart
		stats.End = TimeFixedPoint(float64(stats.Nsec) / 1000000000.0)
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
		// globally and at this time (statRecord).
		if t.Monitor.Enabled != "true" || !statRecord {
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
			if currentType == "kernel" && monitorWindow.Uinsns > 0 {
				monitorError = "non-zero user instructions during kernel operation"
			} else if t.Monitor.Kernel.EnableMin == "true" &&
				float64(monitorWindow.Kinsns)/float64(monitorWindow.Insns) < t.Monitor.Kernel.Min {
				monitorError = "insufficient kernel instructions (potential deadlock)"
			} else if float64(monitorWindow.Kinsns)/float64(monitorWindow.Insns) > t.Monitor.Kernel.Max {
				monitorError = "too many kernel instructions (potential livelock)"
			} else if currentType == "user" && t.Monitor.User.EnableMin == "true" &&
				(float64(monitorWindow.Uinsns)/float64(monitorWindow.Insns) < t.Monitor.User.Min) {
				monitorError = "insufficient user instructions"
			} else if currentType == "user" &&
				(float64(monitorWindow.Uinsns)/float64(monitorWindow.Insns) > t.Monitor.User.Max) {
				monitorError = "too many user instructions"
			}
		}

		// Before we blow up check to make sure that we haven't moved on to a
		// different command or disabled recording or monitoring while we've been
		// calculating.
		if monitorError != "" {
			t.statCond.L.Lock()
			blowup := t.statRecord
			t.statCond.L.Unlock()
			if blowup {
				t.L.Lock()
				blowup = blowup && (currentCounter == t.commandCounter)
				t.L.Unlock()
				if blowup {
					t.stopStats("monitor", monitorError, nil)
					return
				}
			}
		}
	}
}

// enableStats enables stats collection
func (t *Test) enableStats() (bool, error) {
	t.statCond.L.Lock()
	defer t.statCond.L.Unlock()
	if !t.statActive {
		return false, t.statErr
	}
	t.statRecord = true
	t.statCond.Wait()
	return true, nil
}

// disableStats disables stats collection
func (t *Test) disableStats() (bool, error) {
	t.statCond.L.Lock()
	defer t.statCond.L.Unlock()
	if !t.statActive {
		return false, t.statErr
	}
	t.statRecord = false
	return true, nil
}
