package janitor

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/monitor"
	"github.com/rushikesh/devhub/internal/registry"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

type Port struct {
	Port     int    `json:"port"`
	PID      int32  `json:"pid"`
	Cmd      string `json:"cmd"`
	Owner    string `json:"owner"` // session ref or "external"
	Project  string `json:"project,omitempty"`
	Name     string `json:"name,omitempty"`
	SessionID int64 `json:"session_id,omitempty"`
}

type Orphan struct {
	PID     int32  `json:"pid"`
	Name    string `json:"name"`
	Cwd     string `json:"cwd"`
	CPUPct  float64 `json:"cpu_pct"`
	RSS     uint64 `json:"rss_bytes"`
	Age     string `json:"age"`
	Project string `json:"project"`
}

var denyNames = map[string]bool{
	"launchd": true, "WindowServer": true, "kernel_task": true, "init": true,
}

func CanKill(pid int32, name string, uid, selfUID uint32) error {
	if pid < 500 {
		return fmt.Errorf("refusing to kill pid %d (< 500)", pid)
	}
	if denyNames[name] {
		return fmt.Errorf("refusing to kill %s", name)
	}
	if selfUID != 0 && uid != selfUID {
		return fmt.Errorf("refusing to kill pid %d (uid %d != %d)", pid, uid, selfUID)
	}
	return nil
}

func SelfUID() uint32 {
	u, err := user.Current()
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseUint(u.Uid, 10, 32)
	return uint32(n)
}

func ListPorts(sessions []registry.Session, src monitor.ProcSource) ([]Port, error) {
	conns, err := net.Connections("tcp")
	if err != nil || len(conns) == 0 {
		return listPortsLsof(sessions, src)
	}
	var out []Port
	seen := map[int]bool{}
	for _, c := range conns {
		if c.Status != "LISTEN" {
			continue
		}
		p := int(c.Laddr.Port)
		if p == 0 || seen[p] {
			continue
		}
		seen[p] = true
		owner, proj, name, sid := matchSession(int32(c.Pid), sessions, src)
		cmd := procName(int32(c.Pid))
		out = append(out, Port{Port: p, PID: int32(c.Pid), Cmd: cmd, Owner: owner, Project: proj, Name: name, SessionID: sid})
	}
	return out, nil
}

func listPortsLsof(sessions []registry.Session, src monitor.ProcSource) ([]Port, error) {
	cmd := exec.Command("lsof", "-nP", "-iTCP", "-sTCP:LISTEN")
	b, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var out []Port
	seen := map[int]bool{}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 9 || fields[0] == "COMMAND" {
			continue
		}
		pid, _ := strconv.Atoi(fields[1])
		addr := fields[len(fields)-2]
		_, portStr, ok := strings.Cut(addr, ":")
		if !ok {
			continue
		}
		port, _ := strconv.Atoi(portStr)
		if port == 0 || seen[port] {
			continue
		}
		seen[port] = true
		owner, proj, name, sid := matchSession(int32(pid), sessions, src)
		out = append(out, Port{Port: port, PID: int32(pid), Cmd: fields[0], Owner: owner, Project: proj, Name: name, SessionID: sid})
	}
	return out, nil
}

func matchSession(pid int32, sessions []registry.Session, src monitor.ProcSource) (owner, project, name string, id int64) {
	if src == nil {
		src = monitor.GopsutilSource{}
	}
	for _, s := range sessions {
		if s.PanePID <= 0 {
			continue
		}
		tree := monitor.WalkTree(src, int32(s.PanePID))
		for _, t := range tree {
			if t == pid {
				return fmt.Sprintf("%s/%s", s.Project, s.Name), s.Project, s.Name, s.ID
			}
		}
		// pid might be parent of pane? also check if pane is descendant of pid
		ptree := monitor.WalkTree(src, pid)
		for _, t := range ptree {
			if t == int32(s.PanePID) {
				return fmt.Sprintf("%s/%s", s.Project, s.Name), s.Project, s.Name, s.ID
			}
		}
	}
	return "external", "", "", 0
}

func procName(pid int32) string {
	p, err := process.NewProcess(pid)
	if err != nil {
		return ""
	}
	n, _ := p.Name()
	return n
}

func ListOrphans(cfg *config.Config, sessions []registry.Session, src monitor.ProcSource) ([]Orphan, error) {
	if src == nil {
		src = monitor.GopsutilSource{}
	}
	owned := map[int32]bool{}
	for _, s := range sessions {
		if s.PanePID > 0 {
			for _, p := range monitor.WalkTree(src, int32(s.PanePID)) {
				owned[p] = true
			}
		}
	}
	names := map[string]bool{}
	for _, n := range cfg.Janitor.ProcessNames {
		names[strings.ToLower(n)] = true
	}
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}
	var out []Orphan
	for _, p := range procs {
		if p == nil {
			continue
		}
		ppid, err := p.Ppid()
		if err != nil || ppid != 1 {
			continue
		}
		n, _ := p.Name()
		if !names[strings.ToLower(n)] {
			continue
		}
		if owned[p.Pid] {
			continue
		}
		cwd, _ := p.Cwd()
		proj := projectForCwd(cfg, cwd)
		if proj == "" {
			continue
		}
		cpu, _ := p.CPUPercent()
		mem, _ := p.MemoryInfo()
		var rss uint64
		if mem != nil {
			rss = mem.RSS
		}
		ct, _ := p.CreateTime()
		age := time.Since(time.UnixMilli(ct)).Truncate(time.Second).String()
		out = append(out, Orphan{PID: p.Pid, Name: n, Cwd: cwd, CPUPct: cpu, RSS: rss, Age: age, Project: proj})
	}
	return out, nil
}

func projectForCwd(cfg *config.Config, cwd string) string {
	if cwd == "" {
		return ""
	}
	for name, p := range cfg.Projects {
		abs, err := config.ExpandPath(p)
		if err != nil {
			continue
		}
		if cwd == abs || strings.HasPrefix(cwd, abs+string(os.PathSeparator)) {
			return name
		}
	}
	return ""
}

func KillTree(pid int32) error {
	p, err := process.NewProcess(pid)
	if err != nil {
		return err
	}
	name, _ := p.Name()
	uids, _ := p.Uids()
	var uid uint32
	if len(uids) > 0 {
		uid = uint32(uids[0])
	}
	if err := CanKill(pid, name, uid, SelfUID()); err != nil {
		return err
	}
	_ = p.SendSignal(syscall.SIGTERM)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if running, _ := p.IsRunning(); !running {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return p.Kill()
}

func FreePort(port int, sessions []registry.Session, src monitor.ProcSource) error {
	ports, err := ListPorts(sessions, src)
	if err != nil {
		return err
	}
	for _, p := range ports {
		if p.Port == port {
			return KillTree(p.PID)
		}
	}
	return fmt.Errorf("nothing listening on :%d", port)
}

func PortTaken(port int, sessions []registry.Session, src monitor.ProcSource) (Port, bool) {
	ports, err := ListPorts(sessions, src)
	if err != nil {
		return Port{}, false
	}
	for _, p := range ports {
		if p.Port == port {
			return p, true
		}
	}
	return Port{}, false
}

func Adopt(db *registry.DB, project, name string, pid int32, cwd, cmd string) (registry.Session, error) {
	if name == "" {
		name = fmt.Sprintf("orphan-%d", pid)
	}
	row := registry.Session{
		Project: project, Name: name, Kind: registry.KindShadow,
		Command: cmd, Cwd: cwd, PanePID: int(pid), Status: registry.StatusRunning,
		CreatedAt: time.Now(),
	}
	id, err := db.Insert(row)
	if err != nil {
		return registry.Session{}, err
	}
	row.ID = id
	return row, nil
}
