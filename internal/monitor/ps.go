package monitor

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
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
	Start  int64
}

// DefaultSource is a CGO-free ProcSource backed by `ps` (and lsof for cwd).
type DefaultSource struct {
	mu   sync.Mutex
	snap []Proc
	at   time.Time
}

type GopsutilSource = DefaultSource

var (
	psMu     sync.Mutex
	psStderr string
)

func LastPSStderr() string {
	psMu.Lock()
	defer psMu.Unlock()
	return psStderr
}

func psArgs() []string {
	elapsed := "etimes="
	if runtime.GOOS == "darwin" {
		elapsed = "etime="
	}
	return []string{"-axo", "pid=,ppid=,uid=,rss=,pcpu=," + elapsed + ",comm="}
}

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
	procs, _ := ReadPSErr()
	return procs
}

func ReadPSErr() ([]Proc, error) {
	cmd := exec.Command("ps", psArgs()...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	psMu.Lock()
	psStderr = strings.TrimSpace(stderr.String())
	psMu.Unlock()
	procs := ParsePS(stdout.Bytes())
	if len(procs) == 0 && err != nil {
		msg := psStderr
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("ps: %s", msg)
	}
	return procs, nil
}

func splitPS(line string, n int) []string {
	s := strings.TrimSpace(line)
	if s == "" {
		return nil
	}
	out := make([]string, 0, n)
	for i := 0; i < n-1; i++ {
		s = strings.TrimLeft(s, " \t")
		if s == "" {
			return out
		}
		sp := strings.IndexAny(s, " \t")
		if sp < 0 {
			out = append(out, s)
			return out
		}
		out = append(out, s[:sp])
		s = s[sp:]
	}
	s = strings.TrimLeft(s, " \t")
	if s != "" {
		out = append(out, s)
	}
	return out
}

func ParseElapsed(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	var days int64
	rest := s
	if i := strings.IndexByte(s, '-'); i >= 0 {
		d, err := strconv.ParseInt(s[:i], 10, 64)
		if err != nil {
			return 0
		}
		days = d
		rest = s[i+1:]
	}
	parts := strings.Split(rest, ":")
	var h, m, sec int64
	switch len(parts) {
	case 2:
		m, _ = strconv.ParseInt(parts[0], 10, 64)
		sec, _ = strconv.ParseInt(parts[1], 10, 64)
	case 3:
		h, _ = strconv.ParseInt(parts[0], 10, 64)
		m, _ = strconv.ParseInt(parts[1], 10, 64)
		sec, _ = strconv.ParseInt(parts[2], 10, 64)
	default:
		return 0
	}
	return days*86400 + h*3600 + m*60 + sec
}

func ParsePS(b []byte) []Proc {
	var out []Proc
	now := time.Now().Unix()
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		fields := splitPS(sc.Text(), 7)
		if len(fields) < 6 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		ppid, _ := strconv.Atoi(fields[1])
		uid, _ := strconv.Atoi(fields[2])
		rssKB, _ := strconv.ParseUint(fields[3], 10, 64)
		cpu, _ := strconv.ParseFloat(fields[4], 64)
		name := fields[5]
		var start int64
		if len(fields) >= 7 {
			etimes := ParseElapsed(fields[5])
			name = fields[6]
			if etimes > 0 {
				start = now - etimes
			}
		}
		if pid <= 0 {
			continue
		}
		out = append(out, Proc{
			PID: int32(pid), PPID: int32(ppid), UID: int32(uid),
			RSS: rssKB * 1024, CPUPct: cpu, Name: name, Start: start,
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

func (s *DefaultSource) StartTime(pid int32) (int64, error) {
	for _, p := range s.refresh() {
		if p.PID == pid {
			return p.Start, nil
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
