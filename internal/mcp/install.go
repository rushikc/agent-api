package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

func InstallCursor(exe string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	existing := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		bak := path + ".devhub.bak." + time.Now().Format("20060102150405")
		_ = os.WriteFile(bak, b, 0o644)
		_ = json.Unmarshal(b, &existing)
	}
	servers, _ := existing["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers["devhub"] = map[string]any{
		"command": exe,
		"args":    []string{"mcp"},
	}
	existing["mcpServers"] = servers
	b, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func MustExe() string {
	p, err := os.Executable()
	if err != nil {
		return "devhub"
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return p
}

