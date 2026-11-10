package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rushikesh/devhub/internal/registry"
)

func TestArgvRoundTrip(t *testing.T) {
	cases := [][]string{
		{"node", "-e", `setInterval(()=>console.log("tick"),500); setTimeout(()=>{},3600e3)`},
		{"sh", "-c", "echo a && echo b"},
		{"sh", "-c", "echo foo | cat"},
		{"env", "FOO=$HOME", "printenv", "FOO"},
		{"echo", `he said "hi"`},
		{"echo", `trailing\`},
		{"echo", "unicøde", "✓"},
		{"printf", "a\nb"},
	}
	for _, argv := range cases {
		raw := EncodeArgv(argv)
		got := ParseArgv(registry.Session{ArgvJSON: raw, Command: DisplayCommand(argv)})
		if len(got) != len(argv) {
			t.Fatalf("%v vs %v", got, argv)
		}
		for i := range argv {
			if got[i] != argv[i] {
				t.Fatalf("%d %q vs %q", i, got[i], argv[i])
			}
		}
	}
}

func TestWrapCommandOmitsUserArgv(t *testing.T) {
	svc, fake := testService(t)
	argv := []string{"node", "-e", `setTimeout(()=>{}, 1); console.log("x")`}
	sess, err := svc.NewShadow("krayon", "selftest", argv)
	if err != nil {
		t.Fatal(err)
	}
	blob, _ := json.Marshal(fake.Calls)
	if strings.Contains(string(blob), "setTimeout") {
		t.Fatalf("tmux saw user command: %s", blob)
	}
	got := ParseArgv(sess)
	if len(got) != len(argv) || got[2] != argv[2] {
		t.Fatalf("%v", got)
	}
	if err := svc.Restart("krayon/selftest"); err != nil {
		t.Fatal(err)
	}
	again := ParseArgv(mustGet(t, svc, sess.ID))
	if len(again) != len(argv) || again[2] != argv[2] {
		t.Fatalf("restart argv %v", again)
	}
}

func TestShellArgvAndParseFallback(t *testing.T) {
	cmds := []string{
		"echo a; echo b",
		"echo a && echo b",
		"echo foo | cat",
		`echo "quoted"`,
		"echo $HOME",
		"printf 'a\nb'",
	}
	for _, c := range cmds {
		got := ResolveArgv(nil, c)
		want := []string{"/bin/sh", "-c", c}
		if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
			t.Fatalf("%q -> %v", c, got)
		}
		parsed := ParseArgv(registry.Session{Command: c})
		if len(parsed) != 3 || parsed[2] != c {
			t.Fatalf("parse %v", parsed)
		}
		argv := []string{"/bin/echo", "ok"}
		if g := ResolveArgv(argv, c); len(g) != 2 || g[0] != "/bin/echo" {
			t.Fatalf("argv should win %v", g)
		}
	}
}

func TestNoStringsFieldsOnUserCommands(t *testing.T) {
	root := filepath.Join("..")
	files := []string{
		filepath.Join(root, "api", "server.go"),
		filepath.Join(root, "profile", "profile.go"),
		filepath.Join(root, "core", "argv.go"),
		filepath.Join(root, "mcp", "tools.go"),
		filepath.Join(root, "ui", "shadows.go"),
		filepath.Join(root, "e2e", "runner.go"),
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "strings.Fields") {
			t.Fatalf("%s still uses strings.Fields", f)
		}
	}
}

func TestCLIShadowCommandArgv(t *testing.T) {
	svc, _ := testService(t)
	cmd := "echo a; echo b"
	argv := ResolveArgv(nil, cmd)
	sess, err := svc.NewShadow("krayon", "quote", argv)
	if err != nil {
		t.Fatal(err)
	}
	got := ParseArgv(sess)
	if len(got) != 3 || got[2] != cmd {
		t.Fatalf("%v", got)
	}
}

func mustGet(t *testing.T, svc *Service, id int64) registry.Session {
	t.Helper()
	s, err := svc.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
