package janitor

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/monitor"
)

type recKiller struct {
	ch    map[int32][]int32
	sig   []syscall.Signal
	order []int32
	alive map[int32]bool
	uid   uint32
	name  string
}

func (r *recKiller) Children(pid int32) ([]int32, error) { return r.ch[pid], nil }
func (r *recKiller) Name(pid int32) string               { return r.name }
func (r *recKiller) UID(pid int32) uint32                { return r.uid }
func (r *recKiller) Signal(pid int32, sig syscall.Signal) error {
	r.sig = append(r.sig, sig)
	r.order = append(r.order, pid)
	if sig == syscall.SIGKILL || sig == syscall.SIGTERM {
		r.alive[pid] = false
	}
	return nil
}
func (r *recKiller) Running(pid int32) bool { return r.alive[pid] }

func TestKillTreeSignalsChildrenFirst(t *testing.T) {
	k := &recKiller{
		ch:    map[int32][]int32{900: {901, 902}, 901: {903}},
		alive: map[int32]bool{900: true, 901: true, 902: true, 903: true},
		uid:   SelfUID(),
		name:  "node",
	}
	if err := KillTreeWith(900, k, 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if len(k.order) < 4 {
		t.Fatalf("order %+v", k.order)
	}
	if k.order[len(k.order)-1] != 900 && k.order[3] != 900 {
		// last TERM should be root after descendants
		foundRootLast := false
		for i, p := range k.order {
			if p == 900 && i >= 3 {
				foundRootLast = true
			}
		}
		if !foundRootLast {
			t.Fatalf("root should be last among first pass: %+v", k.order)
		}
	}
	if k.order[0] == 900 {
		t.Fatalf("children first, got %+v", k.order)
	}
}

func TestProjectForCwdSymlinkPair(t *testing.T) {
	dir := t.TempDir()
	real := dir + "/real"
	link := dir + "/link"
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Projects = map[string]string{"x": link}
	if got := projectForCwd(cfg, real); got != "x" {
		t.Fatalf("got %q", got)
	}
	if got := projectForCwd(cfg, real+"/sub"); got != "x" {
		t.Fatalf("sub %q", got)
	}
}

func TestListOrphansTmuxReparent(t *testing.T) {
	cfg := config.Default()
	cfg.Projects = map[string]string{"x": "/tmp/x"}
	cfg.Janitor.ProcessNames = []string{"node"}
	procs := []monitor.Proc{{
		PID: 99, PPID: 42, Name: "node", Cwd: "/private/tmp/x",
	}}
	orph, err := ListOrphansAt(cfg, nil, treeSrc{ch: map[int32][]int32{}}, 42, procs)
	if err != nil {
		t.Fatal(err)
	}
	if len(orph) != 1 || orph[0].PID != 99 || orph[0].Project != "x" {
		t.Fatalf("%+v", orph)
	}
}
