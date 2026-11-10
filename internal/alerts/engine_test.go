package alerts

import (
	"testing"
	"time"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/events"
)

type recNote struct{ n []Notification }

func (r *recNote) Name() string { return "macos" }
func (r *recNote) Notify(n Notification) error {
	r.n = append(r.n, n)
	return nil
}

func TestDedupe(t *testing.T) {
	rec := &recNote{}
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	e := New(config.AlertsConfig{Rules: []config.AlertRule{{On: events.ShadowCrashed, Notify: []string{"macos"}}}}, nil)
	e.Notifiers["macos"] = rec
	e.Now = func() time.Time { return now }
	ev := events.Event{Type: events.ShadowCrashed, Target: "p/web"}
	e.Handle(ev)
	e.Handle(ev)
	if len(rec.n) != 1 {
		t.Fatalf("dedupe: %d", len(rec.n))
	}
	now = now.Add(61 * time.Second)
	e.Handle(ev)
	if len(rec.n) != 2 {
		t.Fatalf("after window: %d", len(rec.n))
	}
}

func TestQuietHours(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	rec := &recNote{}
	e := New(config.AlertsConfig{
		QuietHours: config.QuietHours{Start: "23:00", End: "08:00", Location: "Asia/Kolkata"},
		Rules:      []config.AlertRule{{On: events.E2EFailed, Notify: []string{"macos"}}},
	}, nil)
	e.Notifiers["macos"] = rec
	e.Now = func() time.Time { return time.Date(2026, 6, 1, 23, 30, 0, 0, loc) }
	e.Handle(events.Event{Type: events.E2EFailed, Target: "1"})
	if len(rec.n) != 0 {
		t.Fatal("expected quiet")
	}
	e.Now = func() time.Time { return time.Date(2026, 6, 1, 10, 0, 0, 0, loc) }
	e.Handle(events.Event{Type: events.E2EFailed, Target: "2"})
	if len(rec.n) != 1 {
		t.Fatal(rec.n)
	}
}

func TestQuietWrap(t *testing.T) {
	if !quiet(config.QuietHours{Start: "23:00", End: "08:00"}, time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)) {
		t.Fatal("1am should be quiet if local is UTC")
	}
}
