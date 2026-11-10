package monitor

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"github.com/rushikesh/devhub/internal/registry"
)

type ProcSource interface {
	Children(pid int32) ([]int32, error)
	CPUTimes(pid int32) (float64, error)
	RSS(pid int32) (uint64, error)
}

type Sample struct {
	PIDs     []int32
	CPUPct   float64
	RSSBytes uint64
}

type Sampler struct {
	src  ProcSource
	mu   sync.Mutex
	last map[int32]cpuPoint
}

type cpuPoint struct {
	cpu  float64
	wall time.Time
}

func NewSampler(src ProcSource) *Sampler {
	if src == nil {
		src = &DefaultSource{}
	}
	return &Sampler{src: src, last: map[int32]cpuPoint{}}
}

func WalkTree(src ProcSource, root int32) []int32 {
	if root <= 0 {
		return nil
	}
	seen := map[int32]bool{}
	var walk func(int32)
	walk = func(pid int32) {
		if pid <= 0 || seen[pid] {
			return
		}
		seen[pid] = true
		kids, err := src.Children(pid)
		if err != nil {
			return
		}
		for _, k := range kids {
			walk(k)
		}
	}
	walk(root)
	out := make([]int32, 0, len(seen))
	for pid := range seen {
		out = append(out, pid)
	}
	return out
}

func SumTree(src ProcSource, last map[int32]cpuPoint, now time.Time, root int32) (Sample, map[int32]cpuPoint) {
	next := map[int32]cpuPoint{}
	for k, v := range last {
		next[k] = v
	}
	if root <= 0 {
		return Sample{}, next
	}
	pids := WalkTree(src, root)
	var rss uint64
	var cpu float64
	var used int
	for _, pid := range pids {
		r, err := src.RSS(pid)
		if err == nil {
			rss += r
		}
		ct, err := src.CPUTimes(pid)
		if err != nil {
			continue
		}
		if prev, ok := last[pid]; ok && now.After(prev.wall) {
			dt := now.Sub(prev.wall).Seconds()
			if dt > 0 {
				delta := ct - prev.cpu
				if delta < 0 {
					delta = 0
				}
				cpu += (delta / dt) * 100
				used++
			}
		}
		next[pid] = cpuPoint{cpu: ct, wall: now}
	}
	_ = used
	return Sample{PIDs: pids, CPUPct: cpu, RSSBytes: rss}, next
}

func (s *Sampler) Sample(root int32) Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	sample, next := SumTree(s.src, s.last, time.Now(), root)
	s.last = next
	return sample
}

func (s *Sampler) SampleMany(roots []int32) []Sample {
	out := make([]Sample, len(roots))
	for i, r := range roots {
		out[i] = s.Sample(r)
	}
	return out
}

type SessionStats struct {
	ID       int64   `json:"id"`
	Project  string  `json:"project"`
	Name     string  `json:"name"`
	Kind     string  `json:"kind"`
	Status   string  `json:"status"`
	PIDs     []int32 `json:"pids"`
	CPUPct   float64 `json:"cpu_pct"`
	RSSBytes uint64  `json:"rss_bytes"`
	UptimeS  int64   `json:"uptime_s"`
}

type Snapshot struct {
	UpdatedAt time.Time      `json:"updated_at"`
	Active    int            `json:"active"`
	Shadow    int            `json:"shadow"`
	CPUPct    float64        `json:"cpu_pct"`
	RSSBytes  uint64         `json:"rss_bytes"`
	Sessions  []SessionStats `json:"sessions"`
}

func BuildSnapshot(sessions []registry.Session, sampler *Sampler) Snapshot {
	now := time.Now()
	snap := Snapshot{UpdatedAt: now, Sessions: make([]SessionStats, 0, len(sessions))}
	for _, s := range sessions {
		st := SessionStats{
			ID:      s.ID,
			Project: s.Project,
			Name:    s.Name,
			Kind:    string(s.Kind),
			Status:  string(s.Status),
			UptimeS: int64(now.Sub(s.CreatedAt).Seconds()),
		}
		if s.Status == registry.StatusRunning && s.PanePID > 0 {
			sample := sampler.Sample(int32(s.PanePID))
			st.PIDs = sample.PIDs
			st.CPUPct = sample.CPUPct
			st.RSSBytes = sample.RSSBytes
			snap.CPUPct += sample.CPUPct
			snap.RSSBytes += sample.RSSBytes
		}
		if s.Kind == registry.KindActive && s.Status == registry.StatusRunning {
			snap.Active++
		}
		if s.Kind == registry.KindShadow && s.Status == registry.StatusRunning {
			snap.Shadow++
		}
		snap.Sessions = append(snap.Sessions, st)
	}
	return snap
}

func WriteSnapshot(path string, snap Snapshot) error {
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func ReadSnapshot(path string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, err
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

func FormatShort(snap Snapshot) string {
	return fmt.Sprintf("%d active | %d shadow | %.1f%% | %s", snap.Active, snap.Shadow, snap.CPUPct, formatBytes(snap.RSSBytes))
}

func formatBytes(n uint64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	gb := float64(n) / (1024 * 1024 * 1024)
	if gb >= 1 {
		return fmt.Sprintf("%.1f GB", gb)
	}
	mb := float64(n) / (1024 * 1024)
	if mb >= 1 {
		return fmt.Sprintf("%.0f MB", math.Round(mb))
	}
	return fmt.Sprintf("%.0f KB", float64(n)/1024)
}
