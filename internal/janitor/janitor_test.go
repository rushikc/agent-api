package janitor

import (
	"testing"

	"github.com/rushikesh/devhub/internal/monitor"
	"github.com/rushikesh/devhub/internal/registry"
)

type treeSrc struct {
	ch map[int32][]int32
}

func (t treeSrc) Children(pid int32) ([]int32, error) { return t.ch[pid], nil }
func (t treeSrc) CPUTimes(pid int32) (float64, error) { return 0, nil }
func (t treeSrc) RSS(pid int32) (uint64, error)       { return 0, nil }
func (t treeSrc) StartTime(pid int32) (int64, error)  { return 0, nil }

func TestCanKillDenylist(t *testing.T) {
	if err := CanKill(1, "launchd", 0, 501); err == nil {
		t.Fatal("pid 1")
	}
	if err := CanKill(400, "node", 501, 501); err == nil {
		t.Fatal("pid < 500")
	}
	if err := CanKill(999, "WindowServer", 501, 501); err == nil {
		t.Fatal("WindowServer")
	}
	if err := CanKill(999, "node", 502, 501); err == nil {
		t.Fatal("other user")
	}
	if err := CanKill(999, "node", 501, 501); err != nil {
		t.Fatal(err)
	}
}

func TestMatchSessionWalksTree(t *testing.T) {
	src := treeSrc{ch: map[int32][]int32{100: {101, 102}, 101: {103}}}
	sessions := []registry.Session{{ID: 1, Project: "krayon", Name: "web", PanePID: 100}}
	owner, proj, name, id := matchSession(103, sessions, src)
	if owner != "krayon/web" || proj != "krayon" || name != "web" || id != 1 {
		t.Fatalf("%s %s %s %d", owner, proj, name, id)
	}
	owner, _, _, _ = matchSession(999, sessions, src)
	if owner != "external" {
		t.Fatal(owner)
	}
	_ = monitor.WalkTree
}
