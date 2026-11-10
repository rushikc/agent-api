package alerts

import (
	"time"

	"github.com/rushikesh/devhub/internal/events"
	"github.com/rushikesh/devhub/internal/monitor"
)

func (e *Engine) ObserveSnapshot(bus *events.Bus, snap monitor.Snapshot) {
	if e == nil {
		return
	}
	var rssMB int
	var cpuPct float64
	var hold time.Duration
	for _, r := range e.Cfg.Rules {
		if r.On != events.ResourceThreshold {
			continue
		}
		rssMB = r.When.RSSMB
		cpuPct = r.When.CPUPct
		if r.When.For != "" {
			if d, err := time.ParseDuration(r.When.For); err == nil {
				hold = d
			}
		}
	}
	if rssMB == 0 && cpuPct == 0 {
		return
	}
	if hold == 0 {
		hold = 30 * time.Second
	}
	now := e.Now()
	e.mu.Lock()
	if e.overSince == nil {
		e.overSince = map[string]time.Time{}
	}
	for _, s := range snap.Sessions {
		over := false
		if rssMB > 0 && s.RSSBytes >= uint64(rssMB)*1024*1024 {
			over = true
		}
		if cpuPct > 0 && s.CPUPct >= cpuPct {
			over = true
		}
		key := s.Project + "/" + s.Name
		if !over {
			delete(e.overSince, key)
			continue
		}
		start, ok := e.overSince[key]
		if !ok {
			e.overSince[key] = now
			continue
		}
		if now.Sub(start) >= hold {
			e.mu.Unlock()
			if bus != nil {
				bus.Publish(events.Event{
					Type: events.ResourceThreshold, Target: key, Project: s.Project,
					Payload: map[string]any{"rss_bytes": s.RSSBytes, "cpu_pct": s.CPUPct},
				})
			}
			e.mu.Lock()
			e.overSince[key] = now
		}
	}
	e.mu.Unlock()
}
