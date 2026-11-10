package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rushikesh/devhub/internal/config"
)

func TestCursorSyncUnsyncDoesNotDeleteUnmanaged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := config.Default()
	cfg.Skills.Targets = []string{"cursor"}
	root := filepath.Join(home, ".devhub", "skills")
	h := &Hub{Root: root, Cfg: cfg, Home: home, Adapters: []Adapter{CursorAdapter{Home: home}}}
	if err := h.ensureRepo(); err != nil {
		t.Fatal(err)
	}
	p, err := h.NewSkill("go-style", "global")
	if err != nil {
		t.Fatal(err)
	}
	_ = p
	unmanaged := filepath.Join(home, ".cursor", "skills", "mine")
	if err := os.MkdirAll(unmanaged, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unmanaged, "SKILL.md"), []byte("user"), 0o644); err != nil {
		t.Fatal(err)
	}
	man, err := h.Sync(false)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(home, ".cursor", "skills", "go-style")
	if _, err := os.Lstat(dst); err != nil {
		t.Fatal("expected managed skill", err)
	}
	if err := h.Unsync(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(unmanaged, "SKILL.md")); err != nil {
		t.Fatal("unmanaged file was deleted")
	}
	if _, err := os.Lstat(dst); err == nil {
		t.Fatal("managed file should be gone")
	}
	_ = man
}

func TestProjectRuleWritesMDC(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(home, "proj")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Projects = map[string]string{"krayon": repo}
	cfg.Skills.Targets = []string{"cursor"}
	h := &Hub{Root: filepath.Join(home, "skills"), Cfg: cfg, Home: home, Adapters: []Adapter{CursorAdapter{Home: home}}}
	if err := h.ensureRepo(); err != nil {
		t.Fatal(err)
	}
	if _, err := h.NewRule("go", "project:krayon"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Sync(false); err != nil {
		t.Fatal(err)
	}
	mdc := filepath.Join(repo, ".cursor", "rules", "go.mdc")
	b, err := os.ReadFile(mdc)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(b), "alwaysApply") {
		t.Fatal(string(b))
	}
}

func TestGlobalRuleSkipsMissingProjectDir(t *testing.T) {
	home := t.TempDir()
	cfg := config.Default()
	cfg.Projects = map[string]string{"gone": filepath.Join(home, "missing-proj")}
	cfg.Skills.Targets = []string{"cursor"}
	h := &Hub{Root: filepath.Join(home, "skills"), Cfg: cfg, Home: home, Adapters: []Adapter{CursorAdapter{Home: home}}}
	if err := h.ensureRepo(); err != nil {
		t.Fatal(err)
	}
	if _, err := h.NewRule("always", "global"); err != nil {
		t.Fatal(err)
	}
	man, err := h.Sync(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range man.Files {
		if filepath.Dir(f) == filepath.Join(home, "missing-proj", ".cursor", "rules") {
			t.Fatalf("wrote into missing project: %s", f)
		}
	}
}

func TestNewSkillHonorsProjectScope(t *testing.T) {
	home := t.TempDir()
	h := &Hub{Root: filepath.Join(home, "skills"), Cfg: config.Default(), Home: home}
	if err := h.ensureRepo(); err != nil {
		t.Fatal(err)
	}
	p, err := h.NewSkill("x", "project:krayon")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(b), "project:krayon") {
		t.Fatal(string(b))
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
