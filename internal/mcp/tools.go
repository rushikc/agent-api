package mcp

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/rushikesh/devhub/internal/client"
	"github.com/rushikesh/devhub/internal/config"
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
	b, _ := json.MarshalIndent(ss, "", "  ")
	return string(b), nil
}

func (t *Tools) StartShadow(project, command string) (string, error) {
	if !t.allowMut() {
		return "", fmt.Errorf("mcp.allow_mutations is false")
	}
	s, err := t.C.Create(map[string]any{"project": project, "kind": "shadow", "command": command})
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

func ParseID(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

var _ DaemonClient = (*client.Client)(nil)
