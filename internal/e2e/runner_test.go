package e2e

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/events"
	"github.com/rushikesh/devhub/internal/profile"
	"github.com/rushikesh/devhub/internal/registry"
)

func TestJUnitParse(t *testing.T) {
	xml := `<?xml version="1.0"?>
<testsuites>
  <testsuite>
    <testcase classname="a" name="ok"/>
    <testcase classname="a" name="bad"><failure message="boom">stack</failure></testcase>
    <testcase classname="a" name="skip"><skipped/></testcase>
  </testsuite>
</testsuites>`
	s, err := ParseJUnitBytes([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	if s.Passed != 1 || s.Failed != 1 || s.Skipped != 1 || len(s.Failures) != 1 {
		t.Fatalf("%+v", s)
	}
	if s.Failures[0].Name != "a/bad" {
		t.Fatal(s.Failures[0])
	}
}

func TestLifecycleTeardownOnError(t *testing.T) {
	db, err := registry.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	eng := NewFake()
	eng.FailStart = true
	cfg := config.Default().WithDir(t.TempDir())
	q := NewQueue(cfg, db, eng, nil, nil)
	pr := &profile.Profile{Name: "demo", Path: t.TempDir(), E2E: profile.E2EConfig{
		Image: "demo", Command: "echo hi",
		Services: []profile.E2EService{{Name: "db", Image: "postgres:16"}},
	}}
	req := RunRequest{Project: "demo"}
	run, err := q.Enqueue(req, pr)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	var got registry.E2ERun
	for time.Now().Before(deadline) {
		got, _ = db.GetE2ERun(run.ID)
		if got.Status == "failed" || got.Status == "cancelled" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got.Status != "failed" {
		t.Fatalf("status %s", got.Status)
	}
	eng.Mu.Lock()
	defer eng.Mu.Unlock()
	if len(eng.NetsRemoved) == 0 {
		t.Fatal("expected network teardown")
	}
	if len(eng.Containers) != 0 {
		t.Fatalf("leftover containers %#v", eng.Containers)
	}
}

func TestLifecycleCancel(t *testing.T) {
	db, err := registry.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	eng := NewFake()
	eng.WaitFn = func(ctx context.Context, id string) (int, error) {
		<-ctx.Done()
		return -1, ctx.Err()
	}
	cfg := config.Default().WithDir(t.TempDir())
	q := NewQueue(cfg, db, eng, nil, nil)
	dir := t.TempDir()
	pr := &profile.Profile{Name: "demo", Path: dir, E2E: profile.E2EConfig{Image: "demo", Command: "sleep 30"}}
	run, err := q.Enqueue(RunRequest{Project: "demo"}, pr)
	if err != nil {
		t.Fatal(err)
	}
	var errC error
	for i := 0; i < 50; i++ {
		errC = q.Cancel(run.ID)
		if errC == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if errC != nil {
		t.Fatal(errC)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := db.GetE2ERun(run.ID)
		if got.Status == "cancelled" || got.Status == "failed" {
			eng.Mu.Lock()
			n := len(eng.NetsRemoved)
			eng.Mu.Unlock()
			if n == 0 {
				t.Fatal("network not removed on cancel")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("not cancelled")
}

func TestEnqueueNilEngineFails(t *testing.T) {
	db, err := registry.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	bus := events.NewBus()
	var kinds []string
	bus.Subscribe(func(e events.Event) { kinds = append(kinds, e.Type) })
	q := NewQueue(config.Default().WithDir(t.TempDir()), db, nil, bus, nil)
	_, err = q.Enqueue(RunRequest{Project: "demo"}, &profile.Profile{Name: "demo"})
	if err == nil || err != ErrEngineDown {
		t.Fatalf("err %v", err)
	}
	if len(kinds) != 1 || kinds[0] != events.E2EFailed {
		t.Fatalf("events %v", kinds)
	}
	rows, _ := db.ListE2ERuns("demo", 5)
	if len(rows) != 1 || rows[0].Status != "failed" {
		t.Fatalf("rows %+v", rows)
	}
}

func TestDaemonDoesNotFallbackToFake(t *testing.T) {
	eng, err := NewDocker()
	if err != nil {
		if eng != nil {
			t.Fatal("failed NewDocker must return nil engine")
		}
		return
	}
	_ = eng
}

func TestSummaryJSON(t *testing.T) {
	s := Summary{Passed: 1, Failed: 2}
	b, _ := json.Marshal(s)
	if string(b) == "" {
		t.Fatal("empty")
	}
}
