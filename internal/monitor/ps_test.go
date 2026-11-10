package monitor

import "testing"

func TestParsePS(t *testing.T) {
	in := []byte("  10  1 501  1024  1.5 node\n  11 10 501  2048  0.0 npm\n")
	got := ParsePS(in)
	if len(got) != 2 || got[0].PID != 10 || got[0].PPID != 1 || got[1].PPID != 10 {
		t.Fatalf("%+v", got)
	}
	if got[0].RSS != 1024*1024 {
		t.Fatalf("rss %d", got[0].RSS)
	}
	tree := ParsePSTree(in)
	if len(tree[10]) != 1 || tree[10][0] != 11 {
		t.Fatalf("%+v", tree)
	}
}
