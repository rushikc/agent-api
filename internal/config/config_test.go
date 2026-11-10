package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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

func TestUniqueProjectNameAndRename(t *testing.T) {
	dir := t.TempDir()
	cfg := Default().WithDir(dir)
	cfg.Projects = map[string]string{"app": dir}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if got := cfg.UniqueProjectName("app"); got != "app-2" {
		t.Fatal(got)
	}
	if got := cfg.UniqueProjectName("other"); got != "other" {
		t.Fatal(got)
	}
	other := t.TempDir()
	if err := cfg.AddProject("other", other); err != nil {
		t.Fatal(err)
	}
	if err := cfg.RenameProject("other", "renamed"); err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Projects["other"]; ok {
		t.Fatal("old name")
	}
	name, ok := cfg.ProjectNameForPath(other)
	if !ok || name != "renamed" {
		t.Fatalf("%s %v", name, ok)
	}
}

func TestSaveDoesNotDumpScanDefaults(t *testing.T) {
	dir := t.TempDir()
	a := t.TempDir()
	orig := "prefix: C-Space\nprojects:\n  a: " + a + "\n"
	path := filepath.Join(dir, ConfigName)
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Default().WithDir(dir)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.attachDoc(data)
	cfg.normalize()
	extra := t.TempDir()
	if err := cfg.AddProject("temp", extra); err != nil {
		t.Fatal(err)
	}
	mid, _ := os.ReadFile(path)
	if !strings.Contains(string(mid), "temp:") {
		t.Fatalf("expected new project:\n%s", mid)
	}
	if strings.Contains(string(mid), "scan:") || strings.Contains(string(mid), "depth_desktop") {
		t.Fatalf("dumped scan defaults:\n%s", mid)
	}
	if err := cfg.RemoveProject("temp"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	s := string(got)
	if strings.Contains(s, "scan:") || strings.Contains(s, "depth_desktop") || strings.Contains(s, "include_volumes") {
		t.Fatalf("new keys:\n%s", s)
	}
	if strings.Contains(s, "temp:") {
		t.Fatalf("temp project remained:\n%s", s)
	}
	if !strings.Contains(s, "a:") {
		t.Fatalf("lost original project:\n%s", s)
	}
}

func TestDefaultScanFields(t *testing.T) {
	cfg := Default()
	if cfg.Scan.DepthDesktop != 6 || cfg.Scan.DepthAll != 10 || cfg.Scan.IncludeVolumes {
		t.Fatalf("%+v", cfg.Scan)
	}
	if cfg.ScanCachePath() == "" {
		t.Fatal("cache path")
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
