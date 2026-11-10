package mcp

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rushikesh/devhub/internal/client"
	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/jsonx"
	"github.com/rushikesh/devhub/internal/registry"
)

type DaemonClient interface {
	Sessions(project, kind string) ([]registry.Session, error)
	Create(body map[string]any) (registry.Session, error)
	Restart(id int64) error
	Delete(id int64) error
	Logs(id int64, lines int) (string, error)
	Stats() (json.RawMessage, error)
	Ports() (json.RawMessage, error)
	FreePort(port int) error
	ProfileUp(project string) (json.RawMessage, error)
	ProfileDown(project string) error
	Skills() (json.RawMessage, error)
	Skill(name string) (json.RawMessage, error)
	GCPProjects() (json.RawMessage, error)
	GCPCall(alias, task string, params any, confirm, viaMCP bool) (json.RawMessage, error)
	E2ERun(project string, body map[string]any) (json.RawMessage, error)
	E2EGet(id string) (json.RawMessage, error)
	E2ELogs(id string) (string, error)
	SecretsList() (json.RawMessage, error)
	EventsRecent() (json.RawMessage, error)
	ScanCache() (json.RawMessage, error)
	ScanStart(body map[string]any) (json.RawMessage, error)
	ScanJob(id string) (json.RawMessage, error)
	Projects() (json.RawMessage, error)
	AddProject(name, path string) (json.RawMessage, error)
	RemoveProject(name string, kill bool) error
}

type Tools struct {
	C   DaemonClient
	Cfg *config.Config
}

func (t *Tools) allowMut() bool {
	if t.Cfg == nil {
		return true
	}
	return t.Cfg.MCP.AllowMutations
}

func (t *Tools) allowKill() bool {
	if t.Cfg == nil {
		return false
	}
	return t.Cfg.MCP.AllowPortKill
}

func (t *Tools) ListSessions(project, kind string) (string, error) {
	ss, err := t.C.Sessions(project, kind)
	if err != nil {
		return "", err
	}
	b, err := jsonx.Compact(ss)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (t *Tools) StartShadow(project, command string, argv []string, name string) (string, error) {
	if !t.allowMut() {
		return "", fmt.Errorf("mcp.allow_mutations is false")
	}
	resolved := core.ResolveArgv(argv, command)
	if len(resolved) == 0 {
		return "", fmt.Errorf("argv or command is required")
	}
	body := map[string]any{"project": project, "kind": "shadow", "command": command, "argv": resolved}
	if name != "" {
		body["name"] = name
	}
	s, err := t.C.Create(body)
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(s)
	return string(b), nil
}

func (t *Tools) RestartSession(id int64) (string, error) {
	if !t.allowMut() {
		return "", fmt.Errorf("mcp.allow_mutations is false")
	}
	if err := t.C.Restart(id); err != nil {
		return "", err
	}
	return "ok", nil
}

func (t *Tools) StopSession(id int64) (string, error) {
	if !t.allowMut() {
		return "", fmt.Errorf("mcp.allow_mutations is false")
	}
	if err := t.C.Delete(id); err != nil {
		return "", err
	}
	return "ok", nil
}

func (t *Tools) ReadLogs(id int64, lines int, grep string) (string, error) {
	if lines == 0 {
		lines = 200
	}
	out, err := t.C.Logs(id, lines)
	if err != nil {
		return "", err
	}
	out = Scrub(out)
	if grep != "" {
		var keep []string
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, grep) {
				keep = append(keep, line)
			}
		}
		out = strings.Join(keep, "\n")
	}
	return out, nil
}

func (t *Tools) GetStats() (string, error) {
	b, err := t.C.Stats()
	return string(b), err
}

func (t *Tools) ListPorts() (string, error) {
	b, err := t.C.Ports()
	return string(b), err
}

func (t *Tools) FreePort(port int) (string, error) {
	if !t.allowKill() {
		return "", fmt.Errorf("mcp.allow_port_kill is false")
	}
	if err := t.C.FreePort(port); err != nil {
		return "", err
	}
	return "ok", nil
}

func (t *Tools) ProfileUp(project string) (string, error) {
	if !t.allowMut() {
		return "", fmt.Errorf("mcp.allow_mutations is false")
	}
	b, err := t.C.ProfileUp(project)
	return string(b), err
}

func (t *Tools) ProfileDown(project string) (string, error) {
	if !t.allowMut() {
		return "", fmt.Errorf("mcp.allow_mutations is false")
	}
	if err := t.C.ProfileDown(project); err != nil {
		return "", err
	}
	return "ok", nil
}

func (t *Tools) ListSkills() (string, error) {
	b, err := t.C.Skills()
	return string(b), err
}

func (t *Tools) GetSkill(name string) (string, error) {
	b, err := t.C.Skill(name)
	return string(b), err
}

func (t *Tools) GCPListProjects() (string, error) {
	b, err := t.C.GCPProjects()
	return string(b), err
}

func (t *Tools) GCPCall(alias, task string, params map[string]any) (string, error) {
	b, err := t.C.GCPCall(alias, task, params, false, true)
	if err != nil {
		return "", err
	}
	return Scrub(string(b)), nil
}

func (t *Tools) E2ERun(project, grep, shard string) (string, error) {
	if !t.allowMut() {
		return "", fmt.Errorf("mcp.allow_mutations is false")
	}
	b, err := t.C.E2ERun(project, map[string]any{"grep": grep, "shard": shard})
	return string(b), err
}

func (t *Tools) E2EStatus(id string) (string, error) {
	b, err := t.C.E2EGet(id)
	return string(b), err
}

func (t *Tools) E2EFailures(id string) (string, error) {
	b, err := t.C.E2EGet(id)
	if err != nil {
		return "", err
	}
	var run struct {
		Summary      string `json:"summary"`
		SummaryText  string `json:"summary_text"`
		ArtifactPath string `json:"artifact_path"`
		Status       string `json:"status"`
	}
	_ = json.Unmarshal(b, &run)
	var lines []string
	if run.SummaryText != "" {
		lines = append(lines, run.SummaryText)
	}
	var sum struct {
		Failures []struct {
			Name      string `json:"name"`
			Classname string `json:"classname"`
			Message   string `json:"message"`
		} `json:"failures"`
	}
	if run.Summary != "" && json.Unmarshal([]byte(run.Summary), &sum) == nil {
		for _, f := range sum.Failures {
			n := f.Name
			if f.Classname != "" && n != "" {
				n = f.Classname + "/" + n
			} else if n == "" {
				n = f.Classname
			}
			if f.Message != "" {
				lines = append(lines, n+": "+f.Message)
			} else {
				lines = append(lines, n)
			}
		}
	}
	if run.ArtifactPath != "" {
		lines = append(lines, "artifacts: "+run.ArtifactPath)
	}
	if len(lines) == 0 {
		return string(b), nil
	}
	return strings.Join(lines, "\n"), nil
}

func (t *Tools) E2ELogs(id string) (string, error) {
	out, err := t.C.E2ELogs(id)
	if err != nil {
		return "", err
	}
	return Scrub(out), nil
}

func (t *Tools) SecretsList() (string, error) {
	b, err := t.C.SecretsList()
	return string(b), err
}

func (t *Tools) EventsRecent() (string, error) {
	b, err := t.C.EventsRecent()
	return string(b), err
}

func (t *Tools) ScanProjects(all, refresh bool, paths []string) (string, error) {
	if !refresh && !all && len(paths) == 0 {
		b, err := t.C.ScanCache()
		if err == nil && len(b) > 0 && !strings.Contains(string(b), `"stale":true`) {
			return string(b), nil
		}
	}
	body := map[string]any{"all": all, "refresh": refresh, "paths": paths}
	started, err := t.C.ScanStart(body)
	if err != nil {
		return "", err
	}
	var job struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(started, &job); err != nil || job.ID == "" {
		return string(started), err
	}
	for i := 0; i < 120; i++ {
		raw, err := t.C.ScanJob(job.ID)
		if err != nil {
			return "", err
		}
		var st struct {
			Status string          `json:"status"`
			Result json.RawMessage `json:"result"`
			Error  string          `json:"error"`
		}
		if err := json.Unmarshal(raw, &st); err != nil {
			return string(raw), nil
		}
		if st.Status == "done" {
			if len(st.Result) > 0 {
				return string(st.Result), nil
			}
			return string(raw), nil
		}
		if st.Status == "error" {
			return "", fmt.Errorf("%s", st.Error)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return "", fmt.Errorf("scan timed out")
}

func (t *Tools) AddProject(name, path string) (string, error) {
	if !t.allowMut() {
		return "", fmt.Errorf("mcp.allow_mutations is false")
	}
	b, err := t.C.AddProject(name, path)
	return string(b), err
}

func (t *Tools) RemoveProject(name string, kill bool) (string, error) {
	if !t.allowMut() {
		return "", fmt.Errorf("mcp.allow_mutations is false")
	}
	if err := t.C.RemoveProject(name, kill); err != nil {
		return "", err
	}
	return "ok", nil
}

func ParseID(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

var _ DaemonClient = (*client.Client)(nil)
