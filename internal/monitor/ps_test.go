package monitor

import (
	"strings"
	"testing"
	"time"
)

func TestParsePS(t *testing.T) {
	in := []byte("  10  1 501  1024  1.5 0:00.10 00:01 node\n  11 10 501  2048  0.0 0:00.00 00:01 npm\n")
	got := ParsePS(in)
	if len(got) != 2 || got[0].PID != 10 || got[0].PPID != 1 || got[1].PPID != 10 {
		t.Fatalf("%+v", got)
	}
	if got[0].RSS != 1024*1024 {
		t.Fatalf("rss %d", got[0].RSS)
	}
	if got[0].CPUTime < 0.09 || got[0].CPUTime > 0.11 {
		t.Fatalf("cputime %f", got[0].CPUTime)
	}
	tree := ParsePSTree(in)
	if len(tree[10]) != 1 || tree[10][0] != 11 {
		t.Fatalf("%+v", tree)
	}
}

func TestParseCPUTime(t *testing.T) {
	if got := ParseCPUTime("0:00.12"); got < 0.119 || got > 0.121 {
		t.Fatal(got)
	}
	if got := ParseCPUTime("1:02.45"); got < 62.44 || got > 62.46 {
		t.Fatal(got)
	}
	if got := ParseCPUTime("1:02:03"); got != 3723 {
		t.Fatal(got)
	}
}

func TestParseElapsed(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"05:12", 5*60 + 12},
		{"01:02:03", 1*3600 + 2*60 + 3},
		{"2-03:04:05", 2*86400 + 3*3600 + 4*60 + 5},
		{"12", 12},
	}
	for _, tc := range cases {
		if got := ParseElapsed(tc.in); got != tc.want {
			t.Fatalf("%s got %d want %d", tc.in, got, tc.want)
		}
	}
}

func TestParsePSFixtures(t *testing.T) {
	now := time.Now().Unix()
	macos := []byte("  42  1 501  2048  0.3 0:01.20 05:12 /usr/bin/Google Chrome Helper\n")
	got := ParsePS(macos)
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	if got[0].PID != 42 || got[0].Name != "/usr/bin/Google Chrome Helper" {
		t.Fatalf("%+v", got[0])
	}
	if got[0].RSS != 2048*1024 {
		t.Fatalf("rss %d", got[0].RSS)
	}
	wantStart := now - (5*60 + 12)
	if got[0].Start < wantStart-2 || got[0].Start > wantStart+2 {
		t.Fatalf("start %d want ~%d", got[0].Start, wantStart)
	}

	linux := []byte("  99  1 1000  512  1.0 0:00.50 3723 nginx: worker process\n")
	got = ParsePS(linux)
	if len(got) != 1 || got[0].PID != 99 || !strings.Contains(got[0].Name, "nginx") {
		t.Fatalf("%+v", got)
	}
	if got[0].Start < now-3723-2 || got[0].Start > now-3723+2 {
		t.Fatalf("linux start %d", got[0].Start)
	}

	day := []byte("  7  1 501  100  0.0 0:00.00 2-03:04:05 node\n")
	got = ParsePS(day)
	el := ParseElapsed("2-03:04:05")
	if got[0].Start < now-el-2 || got[0].Start > now-el+2 {
		t.Fatalf("day start %d el %d", got[0].Start, el)
	}

	hms := []byte("  8  1 501  100  0.1 0:00.01 01:02:03 node\n")
	got = ParsePS(hms)
	el = ParseElapsed("01:02:03")
	if got[0].Start < now-el-2 || got[0].Start > now-el+2 {
		t.Fatalf("hms start %d", got[0].Start)
	}
}
