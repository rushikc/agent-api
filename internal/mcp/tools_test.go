package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/registry"
)

type fakeDaemon struct {
	sessions   []registry.Session
	logs       string
	killed     bool
	freed      int
	createBody map[string]any
	e2eJSON    json.RawMessage
}

func (f *fakeDaemon) Sessions(project, kind string) ([]registry.Session, error) {
	return f.sessions, nil
}
func (f *fakeDaemon) Create(body map[string]any) (registry.Session, error) {
	f.createBody = body
	return registry.Session{Name: "dev", Project: body["project"].(string), Command: fmtString(body["command"])}, nil
}

func fmtString(v any) string {
	s, _ := v.(string)
	return s
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
	if len(f.e2eJSON) > 0 {
		return f.e2eJSON, nil
	}
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

func TestListSessionsAndE2EWithExitCode(t *testing.T) {
	code := 3
	f := &fakeDaemon{
		sessions: []registry.Session{{Name: "job", ExitCode: &code, Status: registry.StatusExited}},
		e2eJSON:  json.RawMessage(`{"id":"r1","exit_code":1,"summary":"{\"passed\":2,\"failed\":1,\"failures\":[{\"name\":\"bad\",\"classname\":\"a\",\"message\":\"boom\"}]}","summary_text":"2 passed, 1 failed","artifact_path":"/tmp/art"}`),
	}
	tr := &Tools{C: f, Cfg: config.Default()}
	out, err := tr.ListSessions("", "")
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(out)) {
		t.Fatal(out)
	}
	st, err := tr.E2EStatus("r1")
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(st)) {
		t.Fatal(st)
	}
	fail, err := tr.E2EFailures("r1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fail, "2 passed, 1 failed") || !strings.Contains(fail, "bad") {
		t.Fatal(fail)
	}
}

func TestStartShadowArgvAndCommand(t *testing.T) {
	f := &fakeDaemon{}
	cfg := config.Default()
	cfg.MCP.AllowMutations = true
	tr := &Tools{C: f, Cfg: cfg}
	cmd := "echo a; echo b"
	if _, err := tr.StartShadow("krayon", cmd, nil, ""); err != nil {
		t.Fatal(err)
	}
	argv, _ := f.createBody["argv"].([]string)
	if len(argv) != 3 || argv[2] != cmd {
		t.Fatalf("%v %v", argv, f.createBody)
	}
	if _, err := tr.StartShadow("krayon", "", []string{"/bin/echo", "hi"}, "n"); err != nil {
		t.Fatal(err)
	}
	argv, _ = f.createBody["argv"].([]string)
	if len(argv) != 2 || argv[0] != "/bin/echo" {
		t.Fatalf("%v", argv)
	}
}

func TestSafeTextRecovers(t *testing.T) {
	res, _, err := safeText(func() (string, error) { panic("boom") })
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.IsError || len(res.Content) == 0 {
		t.Fatalf("%+v", res)
	}
}
