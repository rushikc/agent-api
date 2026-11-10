package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	p, err := ExpandPath("~/devhub-test")
	if err != nil {
		t.Fatal(err)
	}
	if p != filepath.Join(home, "devhub-test") {
		t.Fatal(p)
	}
}

func TestProjectPathRequiresDirectory(t *testing.T) {
	cfg := Default()
	cfg.Projects = map[string]string{"krayon": filepath.Join(t.TempDir(), "missing")}
	if _, err := cfg.ProjectPath("krayon"); err == nil {
		t.Fatal("expected error for missing dir")
	}
	dir := t.TempDir()
	cfg.Projects["krayon"] = dir
	got, err := cfg.ProjectPath("krayon")
	if err != nil || got != CanonPath(dir) {
		t.Fatalf("got %s %v", got, err)
	}
}

func TestDefaultV2Fields(t *testing.T) {
	cfg := Default()
	if cfg.API.EnableTCP || cfg.API.TCPPort != 7420 {
		t.Fatalf("api %+v", cfg.API)
	}
	if !cfg.MCP.AllowMutations || cfg.MCP.AllowPortKill {
		t.Fatalf("mcp %+v", cfg.MCP)
	}
	if len(cfg.Skills.Targets) != 1 || cfg.Skills.Targets[0] != "cursor" {
		t.Fatalf("skills %+v", cfg.Skills)
	}
	if len(cfg.Janitor.ProcessNames) == 0 {
		t.Fatal("janitor names")
	}
}

func TestHashColorStable(t *testing.T) {
	if HashColor("krayon") != HashColor("krayon") {
		t.Fatal("hash should be stable")
	}
	if HashColor("a") == HashColor("b") && HashColor("c") == HashColor("d") {
		// palettes can collide; just ensure function returns a palette hex
	}
	if !strings.HasPrefix(HashColor("doc-mate"), "#") {
		t.Fatal(HashColor("doc-mate"))
	}
}
