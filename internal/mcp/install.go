package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func CursorMCPPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cursor", "mcp.json"), nil
}

func MergeCursorConfig(existing map[string]any, exe string) map[string]any {
	if existing == nil {
		existing = map[string]any{}
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
	return existing
}

func InstallCursor(exe string) (string, error) {
	path, err := CursorMCPPath()
	if err != nil {
		return "", err
	}
	_, err = InstallCursorAt(path, exe, false)
	return path, err
}

func InstallCursorAt(path, exe string, dry bool) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && !dry {
		return "", err
	}
	oldB, _ := os.ReadFile(path)
	existing := map[string]any{}
	if len(oldB) > 0 {
		_ = json.Unmarshal(oldB, &existing)
	}
	merged := MergeCursorConfig(existing, exe)
	newB, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return "", err
	}
	newB = append(newB, '\n')
	diff := UnifiedDiff(path, string(oldB), string(newB))
	if dry {
		return diff, nil
	}
	if len(oldB) > 0 {
		bak := path + ".devhub.bak." + time.Now().Format("20060102150405")
		_ = os.WriteFile(bak, oldB, 0o644)
	}
	if err := os.WriteFile(path, newB, 0o644); err != nil {
		return "", err
	}
	return diff, nil
}

func UnifiedDiff(name, oldS, newS string) string {
	if oldS == newS {
		return ""
	}
	oldLines := strings.Split(strings.TrimSuffix(oldS, "\n"), "\n")
	newLines := strings.Split(strings.TrimSuffix(newS, "\n"), "\n")
	if oldS == "" {
		oldLines = nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n+++ %s\n", name, name)
	for _, line := range oldLines {
		fmt.Fprintf(&b, "-%s\n", line)
	}
	for _, line := range newLines {
		fmt.Fprintf(&b, "+%s\n", line)
	}
	return b.String()
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
