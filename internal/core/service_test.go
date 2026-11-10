package core

import (
	"testing"

	"github.com/rushikesh/devhub/internal/config"
)

func TestSanitizeName(t *testing.T) {
	got := sanitizeName([]string{"npm", "run", "dev"})
	if got != "npm-run-dev" {
		t.Fatal(got)
	}
	if sanitizeName(nil) != "shadow" {
		t.Fatal("empty")
	}
}

func TestRenderTmuxConf(t *testing.T) {
	cfg := config.Default()
	s := RenderTmuxConf(cfg, "/usr/local/bin/devhub")
	for _, want := range []string{
		"set-option -g prefix C-Space",
		"bind-key -n M-s display-popup",
		"bind-key -n M-p display-popup",
		"bind-key -n M-1 run-shell",
		"devhub picker",
		"devhub shadows",
		"devhub top",
		"devhub projects",
		"devhub skills-ui",
		"devhub janitor",
		"devhub updown",
		"bind-key -n M-k display-popup",
		"bind-key -n M-o display-popup",
		"bind-key -n M-u display-popup",
		"status --short",
		"/usr/local/bin/devhub",
		"jump 1 --session #{session_name}",
		"status-left",
		"#{@project}",
	} {
		if !contains(s, want) {
			t.Errorf("missing %q in conf:\n%s", want, s)
		}
	}
	if contains(s, "bind-key -n M-b ") || contains(s, "bind-key -n M-f ") || contains(s, "bind-key -n M-d ") {
		t.Fatal("must not bind readline Meta keys")
	}
	if contains(s, `'dev `) || contains(s, `"dev `) {
		t.Fatal("tmux.conf must call devhub, not the dev symlink")
	}
	cfg.Keys.Mode = "prefix"
	p := RenderTmuxConf(cfg, "/usr/local/bin/devhub")
	if contains(p, "bind-key -n M-s") {
		t.Fatal("prefix mode should not emit Option bindings")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}
