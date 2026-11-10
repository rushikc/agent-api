package janitor

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/monitor"
	"github.com/rushikesh/devhub/internal/registry"
)

type Port struct {
	Port      int    `json:"port"`
	PID       int32  `json:"pid"`
	Cmd       string `json:"cmd"`
	Owner     string `json:"owner"` // session ref or "external"
	Project   string `json:"project,omitempty"`
	Name      string `json:"name,omitempty"`
	SessionID int64  `json:"session_id,omitempty"`
}

type Orphan struct {
	PID     int32   `json:"pid"`
	Name    string  `json:"name"`
	Cwd     string  `json:"cwd"`
	CPUPct  float64 `json:"cpu_pct"`
	RSS     uint64  `json:"rss_bytes"`
	Age     string  `json:"age"`
	Project string  `json:"project"`
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
	return listPortsLsof(sessions, src)
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
		src = &monitor.DefaultSource{}
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
	for _, p := range monitor.ReadPS() {
		if p.PID == pid {
			return p.Name
		}
	}
	return ""
}

func ListOrphans(cfg *config.Config, sessions []registry.Session, src monitor.ProcSource) ([]Orphan, error) {
	return ListOrphansAt(cfg, sessions, src, monitor.TmuxServerPID("devhub"), nil)
}

func ListOrphansAt(cfg *config.Config, sessions []registry.Session, src monitor.ProcSource, tmuxPID int32, procs []monitor.Proc) ([]Orphan, error) {
	if src == nil {
		src = &monitor.DefaultSource{}
	}
	if procs == nil {
		procs = monitor.ReadPS()
	}
	owned := map[int32]bool{}
	for _, s := range sessions {
		if s.Status != registry.StatusRunning {
			continue
		}
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
	live := map[int32]bool{}
	for _, x := range procs {
		live[x.PID] = true
	}
	var out []Orphan
	for _, p := range procs {
		if !matchProcessName(p.Name, names) {
			continue
		}
		if owned[p.PID] {
			continue
		}
		parentGone := !live[p.PPID]
		if p.PPID != 1 && (tmuxPID == 0 || p.PPID != tmuxPID) && !parentGone {
			continue
		}
		cwd := p.Cwd
		if cwd == "" {
			cwd = monitor.CwdOf(p.PID)
		}
		proj := projectForCwd(cfg, cwd)
		if proj == "" {
			continue
		}
		out = append(out, Orphan{PID: p.PID, Name: p.Name, Cwd: cwd, CPUPct: p.CPUPct, RSS: p.RSS, Project: proj})
	}
	return out, nil
}

func matchProcessName(name string, names map[string]bool) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return false
	}
	if names[n] {
		return true
	}
	if i := strings.IndexAny(n, " \t"); i >= 0 {
		n = n[:i]
	}
	return names[filepath.Base(n)]
}

func projectForCwd(cfg *config.Config, cwd string) string {
	if cwd == "" {
		return ""
	}
	canon := config.CanonPath(cwd)
	for name, p := range cfg.Projects {
		abs := config.CanonPath(p)
		if abs == "" {
			continue
		}
		if canon == abs || strings.HasPrefix(canon, abs+string(os.PathSeparator)) {
			return name
		}
	}
	return ""
}

type TreeKiller interface {
	Children(pid int32) ([]int32, error)
	Name(pid int32) string
	UID(pid int32) uint32
	Signal(pid int32, sig syscall.Signal) error
	Running(pid int32) bool
}

type liveKiller struct {
	kids  map[int32][]int32
	procs map[int32]monitor.Proc
}

func newLiveKiller() *liveKiller {
	k := &liveKiller{kids: map[int32][]int32{}, procs: map[int32]monitor.Proc{}}
	for _, p := range monitor.ReadPS() {
		k.kids[p.PPID] = append(k.kids[p.PPID], p.PID)
		k.procs[p.PID] = p
	}
	return k
}

func (k *liveKiller) Children(pid int32) ([]int32, error) {
	return k.kids[pid], nil
}

func (k *liveKiller) Name(pid int32) string {
	return k.procs[pid].Name
}

func (k *liveKiller) UID(pid int32) uint32 {
	return uint32(k.procs[pid].UID)
}

func (k *liveKiller) Signal(pid int32, sig syscall.Signal) error {
	return syscall.Kill(int(pid), sig)
}

func (k *liveKiller) Running(pid int32) bool {
	err := syscall.Kill(int(pid), 0)
	return err == nil
}

func KillTree(pid int32) error {
	return KillTreeWith(pid, newLiveKiller(), 10*time.Second)
}

func KillTreeWith(pid int32, k TreeKiller, wait time.Duration) error {
	if k == nil {
		k = newLiveKiller()
	}
	if wait <= 0 {
		wait = 10 * time.Second
	}
	if err := CanKill(pid, k.Name(pid), k.UID(pid), SelfUID()); err != nil {
		return err
	}
	var order []int32
	seen := map[int32]bool{}
	var visit func(int32)
	visit = func(p int32) {
		if p <= 0 || seen[p] {
			return
		}
		seen[p] = true
		kids, err := k.Children(p)
		if err != nil {
			kids = nil
		}
		for _, c := range kids {
			visit(c)
		}
		order = append(order, p)
	}
	visit(pid)
	for _, p := range order {
		_ = k.Signal(p, syscall.SIGTERM)
	}
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		any := false
		for _, p := range order {
			if k.Running(p) {
				any = true
				break
			}
		}
		if !any {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, p := range order {
		if k.Running(p) {
			_ = k.Signal(p, syscall.SIGKILL)
		}
	}
	if rs, ok := k.(interface{ Rescan([]int32) []int32 }); ok {
		for _, p := range rs.Rescan(order) {
			_ = k.Signal(p, syscall.SIGKILL)
		}
	}
	return nil
}

func (k *liveKiller) Rescan(orig []int32) []int32 {
	want := map[int32]bool{}
	for _, p := range orig {
		want[p] = true
	}
	fresh := newLiveKiller()
	tmuxPID := monitor.TmuxServerPID("devhub")
	var extra []int32
	for pid, p := range fresh.procs {
		if !want[pid] {
			continue
		}
		if p.PPID == 1 || (tmuxPID != 0 && p.PPID == tmuxPID) {
			extra = append(extra, pid)
		}
	}
	*k = *fresh
	return extra
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
