package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallIdempotent(t *testing.T) {
	home := t.TempDir()
	devhub := filepath.Join(home, ".devhub")
	zshrc := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(zshrc, []byte("export HISTSIZE=1000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, wrote, _, err := Install(devhub, home)
	if err != nil || !wrote {
		t.Fatalf("first install wrote=%v err=%v", wrote, err)
	}
	body, _ := os.ReadFile(zshrc)
	if strings.Count(string(body), zshrcLine) != 1 {
		t.Fatalf("expected one source line:\n%s", body)
	}
	hook := filepath.Join(devhub, "shell", "devhub.zsh")
	if _, err := os.Stat(hook); err != nil {
		t.Fatal(err)
	}
	_, wrote, _, err = Install(devhub, home)
	if err != nil || wrote {
		t.Fatalf("second install should be no-op wrote=%v err=%v", wrote, err)
	}
	body, _ = os.ReadFile(zshrc)
	if strings.Count(string(body), zshrcLine) != 1 {
		t.Fatalf("duplicate line:\n%s", body)
	}
	backs, _ := filepath.Glob(zshrc + ".devhub.bak.*")
	if len(backs) != 1 {
		t.Fatalf("expected one backup, got %v", backs)
	}
}
