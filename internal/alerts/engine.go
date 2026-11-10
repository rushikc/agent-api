package alerts

import (
	"sync"
	"time"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/events"
	"github.com/rushikesh/devhub/internal/registry"
)

type Notifier interface {
	Name() string
	Notify(n Notification) error
}

type Notification struct {
	Title    string
	Message  string
	Sound    bool
	Execute  string
	Activate string
}

type Engine struct {
	Cfg       config.AlertsConfig
	DB        *registry.DB
	Notifiers map[string]Notifier
	Now       func() time.Time

	mu        sync.Mutex
	last      map[string]time.Time
	overSince map[string]time.Time
	muteUntil time.Time
}

func New(cfg config.AlertsConfig, db *registry.DB) *Engine {
	e := &Engine{
		Cfg:       cfg,
		DB:        db,
		Notifiers: map[string]Notifier{"macos": MacOS{}},
		Now:       time.Now,
		last:      map[string]time.Time{},
		overSince: map[string]time.Time{},
	}
	if cfg.Slack.Webhook != "" {
		e.Notifiers["slack"] = Slack{Webhook: cfg.Slack.Webhook}
	}
	return e
}

func (e *Engine) Subscribe(bus *events.Bus) {
	if bus == nil {
		return
	}
	bus.Subscribe(e.Handle)
}

func (e *Engine) Mute(d time.Duration) {
	e.mu.Lock()
	e.muteUntil = e.Now().Add(d)
	e.mu.Unlock()
}

func (e *Engine) Muted() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.Now().Before(e.muteUntil)
}

func (e *Engine) Handle(ev events.Event) {
	if e.DB != nil {
		payload, _ := jsonBytes(ev.Payload)
		_ = e.DB.InsertEvent(registry.EventRow{
			TS: ev.At, Type: ev.Type, Target: ev.Target, Project: ev.Project, Payload: string(payload),
		})
	}
	if e.Muted() || quiet(e.Cfg.QuietHours, e.Now()) {
		return
	}
	key := ev.Type + "|" + ev.Target
	e.mu.Lock()
	if t, ok := e.last[key]; ok && e.Now().Sub(t) < 60*time.Second {
		e.mu.Unlock()
		return
	}
	e.last[key] = e.Now()
	e.mu.Unlock()

	for _, rule := range e.Cfg.Rules {
		if rule.On != ev.Type {
			continue
		}
		n := Notification{
			Title:   "devhub: " + ev.Type,
			Message: ev.Target,
			Sound:   true,
		}
		if ev.Type == events.ShadowCrashed || ev.Type == events.ShadowRestartedMax {
			n.Execute = tmuxFocus(ev.Target)
		}
		if ev.Type == events.E2EFailed {
			for _, a := range rule.Actions {
				if a == "open_report" {
					if p, _ := ev.Payload["artifact"].(string); p != "" {
						n.Execute = "open " + shellQuote(p)
					}
				}
			}
		}
		for _, name := range rule.Notify {
			if nt, ok := e.Notifiers[name]; ok {
				_ = nt.Notify(n)
			}
		}
	}
}

func jsonBytes(v any) ([]byte, error) {
	if v == nil {
		return []byte("{}"), nil
	}
	return marshal(v)
}
