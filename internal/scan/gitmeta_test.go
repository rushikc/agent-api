package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadHEADBranch(t *testing.T) {
	dir := t.TempDir()
	git := filepath.Join(dir, ".git")
	if err := os.Mkdir(git, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(git, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readHEAD(git); got != "main" {
		t.Fatal(got)
	}
}

func TestReadHEADDetached(t *testing.T) {
	dir := t.TempDir()
	git := filepath.Join(dir, ".git")
	if err := os.Mkdir(git, 0o755); err != nil {
		t.Fatal(err)
	}
	sha := "abcdef1234567890"
	if err := os.WriteFile(filepath.Join(git, "HEAD"), []byte(sha+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readHEAD(git); got != "detached@abcdef1" {
		t.Fatal(got)
	}
}

func TestReadOriginMissing(t *testing.T) {
	dir := t.TempDir()
	git := filepath.Join(dir, ".git")
	if err := os.Mkdir(git, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(git, "config"), []byte("[core]\n\trepositoryformatversion = 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readOrigin(git); got != "" {
		t.Fatal(got)
	}
	if err := os.WriteFile(filepath.Join(git, "config"), []byte("[remote \"origin\"]\n\turl = git@github.com:a/b.git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readOrigin(git); got != "git@github.com:a/b.git" {
		t.Fatal(got)
	}
}

func TestInspectRepoStack(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "devhub.yaml"), []byte("name: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := inspectRepo(dir)
	if r.Branch != "main" || !r.HasDevhubYAML {
		t.Fatalf("%+v", r)
	}
	found := false
	for _, s := range r.Stack {
		if s == "go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("stack %v", r.Stack)
	}
}
