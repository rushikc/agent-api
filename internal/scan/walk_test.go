package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWalkNestedSkipped(t *testing.T) {
	root := t.TempDir()
	outer := filepath.Join(root, "outer")
	inner := filepath.Join(outer, "inner")
	mustMkdirGit(t, outer)
	mustMkdirGit(t, inner)
	res, err := Walk(context.Background(), Options{Roots: []string{root}, Depth: 5, Workers: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Repos) != 1 || res.Repos[0].Name != "outer" {
		t.Fatalf("repos=%v", names(res))
	}
	res, err = Walk(context.Background(), Options{Roots: []string{root}, Depth: 5, Nested: true, Workers: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Repos) != 2 {
		t.Fatalf("nested want 2 got %v", names(res))
	}
}

func TestWalkWorktreeGitFile(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, "gitdir")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "worktree")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: "+gitDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Walk(context.Background(), Options{Roots: []string{root}, Depth: 3, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Repos) != 1 || res.Repos[0].Branch != "feat" {
		t.Fatalf("%+v", res.Repos)
	}
}

func TestWalkSymlinkLoopIgnored(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	if err := os.Mkdir(a, 0o755); err != nil {
		t.Fatal(err)
	}
	mustMkdirGit(t, a)
	if err := os.Symlink(a, filepath.Join(a, "loop")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(root, "back")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	res, err := Walk(ctx, Options{Roots: []string{root}, Depth: 8, Workers: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Repos) != 1 {
		t.Fatalf("symlink loop produced %v", names(res))
	}
}

func TestWalkExcludes(t *testing.T) {
	root := t.TempDir()
	nm := filepath.Join(root, "node_modules", "pkg")
	mustMkdirGit(t, nm)
	keep := filepath.Join(root, "keep")
	mustMkdirGit(t, keep)
	hidden := filepath.Join(root, "skipme")
	mustMkdirGit(t, hidden)
	res, err := Walk(context.Background(), Options{Roots: []string{root}, Depth: 5, Exclude: []string{"skipme"}, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Repos) != 1 || res.Repos[0].Name != "keep" {
		t.Fatalf("%v", names(res))
	}
}

func TestWalkPermissionDeniedCounted(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "secret")
	if err := os.Mkdir(blocked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })
	keep := filepath.Join(root, "ok")
	mustMkdirGit(t, keep)
	res, err := Walk(context.Background(), Options{Roots: []string{root}, Depth: 3, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Repos) != 1 {
		t.Fatalf("repos %v", names(res))
	}
	if res.Denied < 1 {
		if _, err := os.ReadDir(blocked); err == nil {
			t.Skip("process can read mode 000 directories")
		}
		t.Fatalf("denied=%d", res.Denied)
	}
}

func TestWalkDepthLimit(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	mustMkdirGit(t, deep)
	res, err := Walk(context.Background(), Options{Roots: []string{root}, Depth: 2, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Repos) != 0 {
		t.Fatalf("depth 2 should miss c, got %v", names(res))
	}
	res, err = Walk(context.Background(), Options{Roots: []string{root}, Depth: 3, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Repos) != 1 {
		t.Fatalf("depth 3 want 1 got %v", names(res))
	}
}

func mustMkdirGit(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func names(res *Result) []string {
	var out []string
	for _, r := range res.Repos {
		out = append(out, r.Name)
	}
	return out
}
