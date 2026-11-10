package events

import (
	"sync"
	"time"
)

const (
	ShadowCrashed      = "shadow.crashed"
	ShadowRestartedMax = "shadow.restarted_max"
	E2EFinished        = "e2e.finished"
	E2EFailed          = "e2e.failed"
	ResourceThreshold  = "resource.threshold"
	PortConflict       = "port.conflict"
	GCPError           = "gcp.error"
)

type Event struct {
	Type    string         `json:"type"`
	Target  string         `json:"target"`
	Project string         `json:"project,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
	At      time.Time      `json:"at"`
}

type Handler func(Event)

type Bus struct {
	mu   sync.RWMutex
	subs []Handler
}

func NewBus() *Bus { return &Bus{} }

func (b *Bus) Subscribe(h Handler) {
	if b == nil || h == nil {
		return
	}
	b.mu.Lock()
	b.subs = append(b.subs, h)
	b.mu.Unlock()
}

func (b *Bus) Publish(e Event) {
	if b == nil {
		return
	}
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	b.mu.RLock()
	subs := append([]Handler(nil), b.subs...)
	b.mu.RUnlock()
	for _, h := range subs {
		h(e)
	}
}
