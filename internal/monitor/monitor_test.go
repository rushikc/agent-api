package monitor

import (
	"testing"
	"time"
)

type fakeProc struct {
	children map[int32][]int32
	cpu      map[int32]float64
	rss      map[int32]uint64
	start    map[int32]int64
}

func (f fakeProc) Children(pid int32) ([]int32, error) {
	return f.children[pid], nil
}
func (f fakeProc) CPUTimes(pid int32) (float64, error) {
	v, ok := f.cpu[pid]
	if !ok {
		return 0, osErr("missing")
	}
	return v, nil
}
func (f fakeProc) RSS(pid int32) (uint64, error) {
	v, ok := f.rss[pid]
	if !ok {
		return 0, osErr("missing")
	}
	return v, nil
}
func (f fakeProc) StartTime(pid int32) (int64, error) {
	if f.start == nil {
		return 1, nil
	}
	return f.start[pid], nil
}

type osErr string

func (e osErr) Error() string { return string(e) }

func TestWalkAndSumTree(t *testing.T) {
	src := fakeProc{
		children: map[int32][]int32{
			1: {2, 3},
			2: {4},
			3: nil,
			4: nil,
		},
		cpu: map[int32]float64{1: 1.0, 2: 2.0, 3: 0.5, 4: 0.5},
		rss: map[int32]uint64{1: 100, 2: 200, 3: 50, 4: 25},
	}
	pids := WalkTree(src, 1)
	if len(pids) != 4 {
		t.Fatalf("tree size %d", len(pids))
	}

	t0 := time.Unix(0, 0)
	_, last := SumTree(src, nil, t0, 1)
	src.cpu = map[int32]float64{1: 1.5, 2: 2.5, 3: 1.0, 4: 1.0} // +0.5s each over 1s wall = 50% * 4 = 200%
	sample, _ := SumTree(src, last, t0.Add(time.Second), 1)
	if sample.RSSBytes != 375 {
		t.Fatalf("rss %d", sample.RSSBytes)
	}
	if sample.CPUPct < 199 || sample.CPUPct > 201 {
		if !(CPUCap() <= 200 && sample.CPUPct == CPUCap()) {
			t.Fatalf("cpu %f cap %f", sample.CPUPct, CPUCap())
		}
	}
}

func TestMissingPID(t *testing.T) {
	src := fakeProc{
		children: map[int32][]int32{},
		cpu:      map[int32]float64{},
		rss:      map[int32]uint64{},
	}
	sample, _ := SumTree(src, nil, time.Now(), 99)
	if sample.CPUPct >= 0 || sample.RSSBytes != 0 {
		t.Fatalf("%+v", sample)
	}
	sample, _ = SumTree(src, nil, time.Now(), 0)
	if len(sample.PIDs) != 0 {
		t.Fatal("pid 0 should skip")
	}
}

func TestCycleSafe(t *testing.T) {
	src := fakeProc{
		children: map[int32][]int32{1: {2}, 2: {1}},
		cpu:      map[int32]float64{1: 1, 2: 1},
		rss:      map[int32]uint64{1: 1, 2: 1},
	}
	pids := WalkTree(src, 1)
	if len(pids) != 2 {
		t.Fatalf("%v", pids)
	}
}

func TestCPUFirstSampleUnknownAndReuse(t *testing.T) {
	src := fakeProc{
		children: map[int32][]int32{10: nil},
		cpu:      map[int32]float64{10: 5},
		rss:      map[int32]uint64{10: 1},
		start:    map[int32]int64{10: 100},
	}
	t0 := time.Unix(0, 0)
	s1, last := SumTree(src, nil, t0, 10)
	if s1.CPUPct >= 0 {
		t.Fatalf("first sample should be unknown, got %f", s1.CPUPct)
	}
	src.cpu[10] = 6
	s2, last := SumTree(src, last, t0.Add(time.Second), 10)
	if s2.CPUPct < 99 || s2.CPUPct > 101 {
		t.Fatalf("second %f", s2.CPUPct)
	}
	src.start[10] = 200
	src.cpu[10] = 50
	s3, last := SumTree(src, last, t0.Add(2*time.Second), 10)
	if s3.CPUPct >= 0 {
		t.Fatalf("reused pid should be unknown, got %f", s3.CPUPct)
	}
	src.cpu[10] = 50.5
	s4, _ := SumTree(src, last, t0.Add(3*time.Second), 10)
	if s4.CPUPct < 0 || s4.CPUPct > CPUCap() {
		t.Fatalf("clamped %f cap %f", s4.CPUPct, CPUCap())
	}
}

func TestFormatShort(t *testing.T) {
	s := FormatShort(Snapshot{Active: 3, Shadow: 2, CPUPct: 12.4, RSSBytes: 1288490188})
	if s != "3 active | 2 shadow | 12.4% | 1.2 GB" {
		t.Fatal(s)
	}
}
