package core

import (
	"strings"
	"testing"

	"github.com/rushikesh/devhub/internal/tmux"
)

func cwdCalls(fake *tmux.Fake, ops ...string) []tmux.Call {
	want := map[string]bool{}
	for _, op := range ops {
		want[op] = true
	}
	var out []tmux.Call
	for _, c := range fake.Calls {
		if want[c.Op] {
			out = append(out, c)
		}
	}
	return out
}

func TestNewActivePassesProjectCwd(t *testing.T) {
	svc, fake := testService(t)
	path, err := svc.Cfg.ProjectPath("krayon")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.NewActive("krayon", ""); err != nil {
		t.Fatal(err)
	}
	calls := cwdCalls(fake, "new-session")
	if len(calls) != 1 {
		t.Fatalf("calls %+v", fake.Calls)
	}
	if calls[0].Cwd != path {
		t.Fatalf("cwd %s want %s", calls[0].Cwd, path)
	}
	if calls[0].Env["DEVHUB_PROJECT"] != "krayon" {
		t.Fatalf("env %+v", calls[0].Env)
	}
	if _, err := svc.NewActive("krayon", ""); err != nil {
		t.Fatal(err)
	}
	wins := cwdCalls(fake, "new-window")
	if len(wins) != 1 || wins[0].Cwd != path {
		t.Fatalf("new-window %+v", wins)
	}
}

func TestNewShadowPassesProjectCwd(t *testing.T) {
	svc, fake := testService(t)
	path, err := svc.Cfg.ProjectPath("krayon")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.NewShadow("krayon", "", []string{"npm", "run", "dev"}); err != nil {
		t.Fatal(err)
	}
	calls := cwdCalls(fake, "new-session")
	if len(calls) != 1 || calls[0].Cwd != path {
		t.Fatalf("%+v", calls)
	}
	if !strings.HasPrefix(calls[0].Session, "dh-shadow-") {
		t.Fatalf("session %s", calls[0].Session)
	}
}

func TestSwitchProjectCreatesThenSwitches(t *testing.T) {
	svc, fake := testService(t)
	path, err := svc.Cfg.ProjectPath("krayon")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SwitchProject("krayon"); err != nil {
		t.Fatal(err)
	}
	var created, switched bool
	for _, c := range fake.Calls {
		if c.Op == "new-session" {
			created = true
			if c.Session != "dh-krayon" || c.Window != "v1" || c.Cwd != path {
				t.Fatalf("create %+v want cwd %s", c, path)
			}
		}
		if c.Op == "switch-client" && c.Target == "dh-krayon" {
			switched = true
		}
	}
	if !created || !switched {
		t.Fatalf("created=%v switched=%v calls=%+v", created, switched, fake.Calls)
	}

	n := len(fake.Calls)
	if err := svc.SwitchProject("krayon"); err != nil {
		t.Fatal(err)
	}
	var extraNew bool
	for _, c := range fake.Calls[n:] {
		if c.Op == "new-session" || c.Op == "new-window" {
			extraNew = true
		}
	}
	if extraNew {
		t.Fatalf("existing project should only switch-client: %+v", fake.Calls[n:])
	}
}

func TestRestartRespawnsWithCwd(t *testing.T) {
	svc, fake := testService(t)
	path, err := svc.Cfg.ProjectPath("krayon")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := svc.NewShadow("krayon", "dev", []string{"npm", "run", "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Restart(sess.Project + "/" + sess.Name); err != nil {
		t.Fatal(err)
	}
	resp := cwdCalls(fake, "respawn-pane")
	if len(resp) != 1 || resp[0].Cwd != path {
		t.Fatalf("respawn %+v", resp)
	}
}
