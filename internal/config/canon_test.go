package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonPathEvalSymlinks(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "foo")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "alias")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if CanonPath(link) != CanonPath(real) {
		t.Fatalf("%s vs %s", CanonPath(link), CanonPath(real))
	}
}

func TestAddProjectSuggestsNearby(t *testing.T) {
	parent := t.TempDir()
	if err := os.Mkdir(filepath.Join(parent, "foo"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := Default().WithDir(t.TempDir())
	err := cfg.AddProject("x", filepath.Join(parent, "fooz"))
	if err == nil {
		t.Fatal("expected missing path")
	}
	if !strings.Contains(err.Error(), "foo") {
		t.Fatal(err)
	}
}

func TestReloadSeesNewProject(t *testing.T) {
	dir := t.TempDir()
	cfg := Default().WithDir(dir)
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	proj := t.TempDir()
	cfg.Projects["krayon"] = proj
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	loaded := Default().WithDir(dir)
	if err := loaded.Reload(); err != nil {
		t.Fatal(err)
	}
	p, err := loaded.ProjectPath("krayon")
	if err != nil {
		t.Fatal(err)
	}
	if p != CanonPath(proj) {
		t.Fatalf("%s vs %s", p, CanonPath(proj))
	}
}
