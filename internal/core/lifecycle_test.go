package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rushikesh/devhub/internal/client"
	"github.com/rushikesh/devhub/internal/registry"
	"github.com/rushikesh/devhub/internal/tmux"
)

func TestCreateSucceedsWithoutDefaultPath(t *testing.T) {
	svc, fake := testService(t)
	if _, err := svc.NewActive("krayon", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.NewShadow("krayon", "web", []string{"npm", "run", "dev"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SwitchProject("krayon"); err != nil {
		t.Fatal(err)
	}
	for _, c := range fake.Calls {
		if c.Op == "default-path" {
			t.Fatalf("default-path must not be called: %+v", fake.Calls)
		}
	}
	var project, color bool
	for _, c := range fake.Calls {
		if c.Op == "set-option" {
			if _, ok := c.Env["@project"]; ok {
				project = true
			}
			if _, ok := c.Env["@color"]; ok {
				color = true
			}
		}
	}
	if !project || !color {
		t.Fatalf("missing @project/@color: %+v", fake.Calls)
	}
}

func TestCreateRollbackKillsWindow(t *testing.T) {
	svc, fake := testService(t)
	fake.OmitPanes = true
	if _, err := svc.NewActive("krayon", "v1"); err == nil {
		t.Fatal("expected pane lookup failure")
	}
	var killed bool
	for _, c := range fake.Calls {
		if c.Op == "kill-window" {
			killed = true
		}
	}
	if !killed {
		t.Fatal("expected kill-window rollback")
	}
	if rows, _ := svc.DB.List(true); len(rows) != 0 {
		t.Fatalf("expected no db row, got %+v", rows)
	}
}

func TestShadowUpsertSameName(t *testing.T) {
	svc, _ := testService(t)
	s1, err := svc.NewShadow("krayon", "web", []string{"npm", "run", "dev"})
	if err != nil {
		t.Fatal(err)
	}
	s2, err := svc.NewShadow("krayon", "web", []string{"npm", "run", "start"})
	if err != nil {
		t.Fatal(err)
	}
	if s1.ID != s2.ID {
		t.Fatalf("expected upsert same id %d vs %d", s1.ID, s2.ID)
	}
	if s2.Command != "npm run start" {
		t.Fatalf("command %s", s2.Command)
	}
	if !strings.Contains(s2.TmuxTarget, "@") {
		t.Fatalf("tmux_target should use window id: %s", s2.TmuxTarget)
	}
}

func TestPeekRestartUsesWindowID(t *testing.T) {
	svc, fake := testService(t)
	sess, err := svc.NewShadow("krayon", "web", []string{"npm", "run", "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sess.TmuxTarget, ":@") {
		t.Fatalf("target %s", sess.TmuxTarget)
	}
	if _, err := svc.Peek("krayon/web"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Restart("krayon/web"); err != nil {
		t.Fatal(err)
	}
	var respawn string
	for _, c := range fake.Calls {
		if c.Op == "respawn-pane" {
			respawn = c.Target
		}
	}
	if respawn != sess.TmuxTarget {
		t.Fatalf("respawn %s want %s", respawn, sess.TmuxTarget)
	}
}

func TestApplyKeybindingsReloadOnStaleHash(t *testing.T) {
	svc, fake := testService(t)
	svc.Cfg.WithDir(t.TempDir())
	if err := os.MkdirAll(svc.Cfg.Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := svc.ApplyKeybindingsForce(true); err != nil {
		t.Fatal(err)
	}
	n := len(fake.Sourced)
	if err := svc.ApplyKeybindings(); err != nil {
		t.Fatal(err)
	}
	if len(fake.Sourced) != n {
		t.Fatal("fresh hash should skip source")
	}
	_ = os.WriteFile(svc.Cfg.TmuxConfPath()+".hash", []byte("stale\n"), 0o644)
	if err := svc.ApplyKeybindings(); err != nil {
		t.Fatal(err)
	}
	if len(fake.Sourced) != n+1 {
		t.Fatalf("stale hash should source again, sourced=%d", len(fake.Sourced))
	}
	body := RenderTmuxConf(svc.Cfg, svc.Executable())
	if !strings.Contains(body, "M-s") {
		t.Fatal(body)
	}
}

func TestNewActiveGoesThroughRemote(t *testing.T) {
	svc, _ := testService(t)
	var got map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/sessions" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(registry.Session{ID: 9, Project: "krayon", Name: "v1", Kind: registry.KindActive})
	}))
	t.Cleanup(ts.Close)
	svc.Remote = &client.Client{HTTP: ts.Client(), Base: ts.URL, Caller: "cli"}
	s, err := svc.NewActive("krayon", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != 9 {
		t.Fatalf("%+v", s)
	}
	if got["project"] != "krayon" || got["kind"] != "active" {
		t.Fatalf("%+v", got)
	}
}

func TestCGODisabled(t *testing.T) {
	if os.Getenv("CGO_ENABLED") == "1" {
		t.Fatal("CGO must be disabled; see go.env and make test")
	}
	_ = filepath.Separator
}

func TestFakeTmuxHasNoDefaultPath(t *testing.T) {
	var _ tmux.Client = tmux.NewFake()
}
