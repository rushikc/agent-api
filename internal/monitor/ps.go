package monitor

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Proc is one row from `ps`.
type Proc struct {
	PID    int32
	PPID   int32
	UID    int32
	RSS    uint64
	CPUPct float64
	Name   string
	Cwd    string
}

// DefaultSource is a CGO-free ProcSource backed by `ps` (and lsof for cwd).
type DefaultSource struct {
	mu   sync.Mutex
	snap []Proc
	at   time.Time
}

type GopsutilSource = DefaultSource

func (s *DefaultSource) refresh() []Proc {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.at) < 200*time.Millisecond && s.snap != nil {
		return s.snap
	}
	s.snap = ReadPS()
	s.at = time.Now()
	return s.snap
}

func ReadPS() []Proc {
	cmd := exec.Command("ps", "-axo", "pid=,ppid=,uid=,rss=,pcpu=,comm=")
	b, err := cmd.Output()
	if err != nil {
		return nil
	}
	return ParsePS(b)
}

func ParsePS(b []byte) []Proc {
	var out []Proc
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 6 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		ppid, _ := strconv.Atoi(fields[1])
		uid, _ := strconv.Atoi(fields[2])
		rssKB, _ := strconv.ParseUint(fields[3], 10, 64)
		cpu, _ := strconv.ParseFloat(fields[4], 64)
		name := fields[5]
		if pid <= 0 {
			continue
		}
		out = append(out, Proc{
			PID: int32(pid), PPID: int32(ppid), UID: int32(uid),
			RSS: rssKB * 1024, CPUPct: cpu, Name: name,
		})
	}
	return out
}

func ParsePSTree(b []byte) map[int32][]int32 {
	kids := map[int32][]int32{}
	for _, p := range ParsePS(b) {
		kids[p.PPID] = append(kids[p.PPID], p.PID)
	}
	return kids
}

func (s *DefaultSource) Children(pid int32) ([]int32, error) {
	var kids []int32
	for _, p := range s.refresh() {
		if p.PPID == pid {
			kids = append(kids, p.PID)
		}
	}
	return kids, nil
}

func (s *DefaultSource) CPUTimes(pid int32) (float64, error) {
	for _, p := range s.refresh() {
		if p.PID == pid {
			return p.CPUPct, nil
		}
	}
	return 0, fmt.Errorf("no pid %d", pid)
}

func (s *DefaultSource) RSS(pid int32) (uint64, error) {
	for _, p := range s.refresh() {
		if p.PID == pid {
			return p.RSS, nil
		}
	}
	return 0, fmt.Errorf("no pid %d", pid)
}

func (s *DefaultSource) List() []Proc {
	return s.refresh()
}

func CwdOf(pid int32) string {
	cmd := exec.Command("lsof", "-a", "-d", "cwd", "-p", strconv.Itoa(int(pid)), "-Fn")
	b, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "n") {
			return strings.TrimPrefix(line, "n")
		}
	}
	return ""
}

func TmuxServerPID(socket string) int32 {
	if socket == "" {
		socket = "devhub"
	}
	cmd := exec.Command("tmux", "-L", socket, "display-message", "-p", "#{pid}")
	b, err := cmd.Output()
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return int32(n)
}
