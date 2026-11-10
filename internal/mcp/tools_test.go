package mcp

import (
	"encoding/json"
	"testing"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/registry"
)

type fakeDaemon struct {
	sessions []registry.Session
	logs     string
	killed   bool
	freed    int
}

func (f *fakeDaemon) Sessions(project, kind string) ([]registry.Session, error) {
	return f.sessions, nil
}
func (f *fakeDaemon) Create(body map[string]any) (registry.Session, error) {
	return registry.Session{Name: "dev", Project: body["project"].(string)}, nil
}
func (f *fakeDaemon) Restart(id int64) error                   { return nil }
func (f *fakeDaemon) Delete(id int64) error                    { f.killed = true; return nil }
func (f *fakeDaemon) Logs(id int64, lines int) (string, error) { return f.logs, nil }
func (f *fakeDaemon) Stats() (json.RawMessage, error)          { return json.RawMessage(`{}`), nil }
func (f *fakeDaemon) Ports() (json.RawMessage, error)          { return json.RawMessage(`[]`), nil }
func (f *fakeDaemon) FreePort(port int) error                  { f.freed = port; return nil }
func (f *fakeDaemon) ProfileUp(project string) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}
func (f *fakeDaemon) ProfileDown(project string) error { return nil }
func (f *fakeDaemon) Skills() (json.RawMessage, error) { return json.RawMessage(`[]`), nil }
func (f *fakeDaemon) Skill(name string) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}
func (f *fakeDaemon) GCPProjects() (json.RawMessage, error) { return json.RawMessage(`[]`), nil }
func (f *fakeDaemon) GCPCall(alias, task string, params any, confirm, viaMCP bool) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}
func (f *fakeDaemon) E2ERun(project string, body map[string]any) (json.RawMessage, error) {
	return json.RawMessage(`{"id":"1"}`), nil
}
func (f *fakeDaemon) E2EGet(id string) (json.RawMessage, error) {
	return json.RawMessage(`{"id":"` + id + `","summary":"{}"}`), nil
}
func (f *fakeDaemon) E2ELogs(id string) (string, error) { return "ok", nil }
func (f *fakeDaemon) SecretsList() (json.RawMessage, error) {
	return json.RawMessage(`[{"name":"x","ok":true}]`), nil
}
func (f *fakeDaemon) EventsRecent() (json.RawMessage, error) { return json.RawMessage(`[]`), nil }

func TestToolsMutationsAndScrub(t *testing.T) {
	f := &fakeDaemon{logs: "AWS_TOKEN=supersecret\nready\n"}
	cfg := config.Default()
	cfg.MCP.AllowMutations = true
	cfg.MCP.AllowPortKill = false
	tr := &Tools{C: f, Cfg: cfg}
	out, err := tr.ReadLogs(1, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if contains(out, "supersecret") {
		t.Fatal(out)
	}
	if _, err := tr.FreePort(3000); err == nil {
		t.Fatal("port kill should be denied")
	}
	if _, err := tr.StopSession(1); err != nil {
		t.Fatal(err)
	}
	if !f.killed {
		t.Fatal("expected delete")
	}
}
