package core

import (
	"path/filepath"
	"testing"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/monitor"
	"github.com/rushikesh/devhub/internal/registry"
	"github.com/rushikesh/devhub/internal/tmux"
)

func testService(t *testing.T) (*Service, *tmux.Fake) {
	t.Helper()
	db, err := registry.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	fake := tmux.NewFake()
	cfg := config.Default()
	cfg.Projects = map[string]string{"krayon": t.TempDir()}
	return &Service{Cfg: cfg, Tmux: fake, DB: db, Sampler: monitor.NewSampler(nil)}, fake
}

func TestNewActiveAndKill(t *testing.T) {
	svc, fake := testService(t)
	s, err := svc.NewActive("krayon", "")
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "v1" || s.Kind != registry.KindActive {
		t.Fatalf("%+v", s)
	}
	s2, err := svc.NewActive("krayon", "")
	if err != nil {
		t.Fatal(err)
	}
	if s2.Name != "v2" {
		t.Fatalf("expected v2 got %s", s2.Name)
	}
	if err := svc.Kill("v1"); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Lookup("v1")
	if got.Status != registry.StatusExited {
		t.Fatalf("%+v", got)
	}
	if err := svc.Reconcile(); err != nil {
		t.Fatal(err)
	}
	_ = fake
}

func TestLookupAmbiguous(t *testing.T) {
	svc, _ := testService(t)
	svc.Cfg.Projects["other"] = t.TempDir()
	if _, err := svc.NewActive("krayon", "api"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.NewActive("other", "api"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Lookup("api"); err == nil {
		t.Fatal("expected ambiguous")
	}
	got, err := svc.Lookup("krayon/api")
	if err != nil || got.Project != "krayon" {
		t.Fatal(err, got)
	}
}
