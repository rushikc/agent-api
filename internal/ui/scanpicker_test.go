package ui

import (
	"testing"
	"time"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/scan"
)

func TestScanPickerToggleApplyDiff(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default().WithDir(dir)
	cfg.Projects = map[string]string{"keep": "/tmp/keep", "gone": "/tmp/gone"}
	svc := &core.Service{Cfg: cfg}
	res := &scan.Result{Repos: []scan.Repo{
		{Name: "keep", Path: "/tmp/keep", Registered: true, RegisteredName: "keep"},
		{Name: "gone", Path: "/tmp/gone", Registered: true, RegisteredName: "gone"},
		{Name: "new", Path: "/tmp/new"},
		{Name: "clash", Path: "/tmp/clash"},
	}}
	m := newScanModel(svc, res, 0, false)
	// uncheck gone, check new
	for i := range m.rows {
		switch m.rows[i].Repo.Name {
		case "gone":
			m.rows[i].Checked = false
		case "new":
			m.rows[i].Checked = true
		case "clash":
			m.rows[i].Checked = true
			m.rows[i].AddName = "keep" // clash with registered
		}
	}
	d := m.applyDiff()
	if len(d.Removes) != 1 || d.Removes[0] != "gone" {
		t.Fatalf("removes %+v", d.Removes)
	}
	if len(d.Adds) != 2 {
		t.Fatalf("adds %+v", d.Adds)
	}
	names := map[string]string{}
	for _, a := range d.Adds {
		names[a.Path] = a.Name
	}
	if names["/tmp/new"] != "new" {
		t.Fatalf("new name %v", names)
	}
	if names["/tmp/clash"] != "keep-2" {
		t.Fatalf("clash suffix %v", names)
	}
}

func TestScanPickerRenameDiff(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default().WithDir(dir)
	cfg.Projects = map[string]string{"old": "/tmp/old"}
	svc := &core.Service{Cfg: cfg}
	res := &scan.Result{Repos: []scan.Repo{
		{Name: "old", Path: "/tmp/old", Registered: true, RegisteredName: "old"},
	}}
	m := newScanModel(svc, res, 0, false)
	m.rows[0].AddName = "fresh"
	d := m.applyDiff()
	if len(d.Renames) != 1 || d.Renames[0].Old != "old" || d.Renames[0].New != "fresh" {
		t.Fatalf("%+v", d)
	}
}

func TestScanPickerFilterSortTab(t *testing.T) {
	cfg := config.Default()
	svc := &core.Service{Cfg: cfg}
	res := &scan.Result{Repos: []scan.Repo{
		{Name: "zeta", Path: "/z/zeta", LastActivity: time.Now().Add(-time.Hour), Registered: true, RegisteredName: "zeta"},
		{Name: "alpha", Path: "/a/alpha", LastActivity: time.Now()},
	}}
	m := newScanModel(svc, res, time.Minute, true)
	m.filter = "alp"
	vis := m.visible()
	if len(vis) != 1 || m.rows[vis[0]].Repo.Name != "alpha" {
		t.Fatalf("filter %v", vis)
	}
	m.filter = ""
	m.tab = scanTabAdded
	vis = m.visible()
	if len(vis) != 1 || m.rows[vis[0]].Repo.Name != "zeta" {
		t.Fatalf("tab added %v", vis)
	}
	m.tab = scanTabNotAdded
	vis = m.visible()
	if len(vis) != 1 || m.rows[vis[0]].Repo.Name != "alpha" {
		t.Fatalf("tab not %v", vis)
	}
	m.tab = scanTabAll
	m.sort = scanSortPath
	m.applySort()
	if m.rows[0].Repo.Name != "alpha" {
		t.Fatalf("sort path %s", m.rows[0].Repo.Name)
	}
	m.sort = scanSortActivity
	m.applySort()
	if m.rows[0].Repo.Name != "alpha" {
		t.Fatalf("sort activity %s", m.rows[0].Repo.Name)
	}
	m.toggleAllVisible()
	if !m.rows[0].Checked || !m.rows[1].Checked {
		t.Fatal("toggle all")
	}
}

func TestFuzzyMatch(t *testing.T) {
	if !fuzzyMatch("agt", "agent-api") {
		t.Fatal("subsequence")
	}
	if fuzzyMatch("xyz", "agent") {
		t.Fatal("should miss")
	}
}
