package process

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type ClaudeInfo struct {
	Running        bool
	WaitingForInput bool  // true when Claude is waiting for user response
	CPUSecs        int64  // cumulative CPU seconds
	CPUTime        string // formatted (H:MM:SS or M:SS)
	PID            int    // claude process PID
	RChar          int64  // cumulative bytes read (for IO delta tracking)
}

func (c ClaudeInfo) Pretty() string {
	if !c.Running {
		return "-"
	}
	if c.WaitingForInput {
		return c.CPUTime + " (waiting)"
	}
	return c.CPUTime
}

// ioTracker stores previous rchar readings per session for delta computation.
var ioTracker = struct {
	sync.Mutex
	prev map[string]int64
}{prev: make(map[string]int64)}

// ioThreshold: bytes read below this in one poll interval means "waiting for input".
const ioThreshold = 500

// GetClaudeInfo checks if claude is running in a zellij session.
// Reads /proc directly - no subprocesses.
func GetClaudeInfo(zellijSession string) ClaudeInfo {
	if zellijSession == "" {
		return ClaudeInfo{}
	}

	serverPID := findZellijServer(zellijSession)
	if serverPID == 0 {
		return ClaudeInfo{}
	}

	claudePID := findClaudeUnder(serverPID)
	if claudePID == 0 {
		return ClaudeInfo{}
	}

	children := getChildren(claudePID)

	utime, stime := readCPUTicks(claudePID)
	for _, child := range children {
		cu, cs := readCPUTicks(child)
		utime += cu
		stime += cs
	}

	totalSecs := (utime + stime) / clockTicks()
	rchar := readRChar(claudePID)

	// Compare with previous reading to detect IO activity
	ioTracker.Lock()
	prevRChar, hasPrev := ioTracker.prev[zellijSession]
	ioTracker.prev[zellijSession] = rchar
	ioTracker.Unlock()

	waiting := false
	if hasPrev {
		delta := rchar - prevRChar
		waiting = delta < ioThreshold
	}

	return ClaudeInfo{
		Running:         true,
		WaitingForInput: waiting,
		CPUSecs:         totalSecs,
		CPUTime:         formatDuration(totalSecs),
		PID:             claudePID,
		RChar:           rchar,
	}
}

func findZellijServer(session string) int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	target := "/" + session
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			continue
		}
		cmd := string(cmdline)
		if strings.Contains(cmd, "zellij") && strings.Contains(cmd, "--server") && strings.Contains(cmd, target) {
			return pid
		}
	}
	return 0
}

func findClaudeUnder(zellijServerPID int) int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(comm)) != "claude" {
			continue
		}
		if isDescendant(pid, zellijServerPID) {
			return pid
		}
	}
	return 0
}

func isDescendant(pid, ancestor int) bool {
	current := pid
	for i := 0; i < 20; i++ {
		ppid := readPPID(current)
		if ppid <= 1 {
			return false
		}
		if ppid == ancestor {
			return true
		}
		current = ppid
	}
	return false
}

func readPPID(pid int) int {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}
	s := string(data)
	idx := strings.LastIndex(s, ")")
	if idx < 0 {
		return 0
	}
	fields := strings.Fields(s[idx+2:])
	if len(fields) < 2 {
		return 0
	}
	ppid, _ := strconv.Atoi(fields[1])
	return ppid
}

func readRChar(pid int) int64 {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/io", pid))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "rchar:") {
			val, _ := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "rchar:")), 10, 64)
			return val
		}
	}
	return 0
}

func readCPUTicks(pid int) (int64, int64) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, 0
	}
	s := string(data)
	idx := strings.LastIndex(s, ")")
	if idx < 0 {
		return 0, 0
	}
	fields := strings.Fields(s[idx+2:])
	if len(fields) < 13 {
		return 0, 0
	}
	utime, _ := strconv.ParseInt(fields[11], 10, 64)
	stime, _ := strconv.ParseInt(fields[12], 10, 64)
	return utime, stime
}

func getChildren(pid int) []int {
	pattern := fmt.Sprintf("/proc/%d/task/*/children", pid)
	matches, _ := filepath.Glob(pattern)
	var children []int
	for _, m := range matches {
		data, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		for _, s := range strings.Fields(string(data)) {
			if cpid, err := strconv.Atoi(s); err == nil {
				children = append(children, cpid)
			}
		}
	}
	return children
}

func clockTicks() int64 {
	return 100
}

func formatDuration(totalSecs int64) string {
	h := totalSecs / 3600
	m := (totalSecs % 3600) / 60
	s := totalSecs % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
