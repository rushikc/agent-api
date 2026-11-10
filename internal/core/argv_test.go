package core

import (
	"encoding/json"
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

func mustGet(t *testing.T, svc *Service, id int64) registry.Session {
	t.Helper()
	s, err := svc.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
