package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/monitor"
	"github.com/rushikesh/devhub/internal/registry"
	"github.com/rushikesh/devhub/internal/scan"
	"github.com/rushikesh/devhub/internal/tmux"
)

func TestProjectsCRUD(t *testing.T) {
	dir := t.TempDir()
	proj := t.TempDir()
	cfg := config.Default().WithDir(dir)
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	db, err := registry.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := &core.Service{Cfg: cfg, Tmux: tmux.NewFake(), DB: db, Sampler: monitor.NewSampler(nil)}
	srv := NewWith(cfg, svc, nil, Deps{Scan: scan.NewRunner(cfg)})

	body, _ := json.Marshal(map[string]string{"name": "demo", "path": proj})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/projects", bytes.NewReader(body))
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("add %d %s", rec.Code, rec.Body.String())
	}
	if _, err := cfg.ProjectPath("demo"); err != nil {
		t.Fatal(err)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/projects", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 || !bytes.Contains(rec.Body.Bytes(), []byte("demo")) {
		t.Fatalf("list %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/v1/projects/demo", bytes.NewReader([]byte(`{"name":"demo2"}`)))
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Fatalf("rename %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/v1/projects/demo2", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Fatalf("delete %d %s", rec.Code, rec.Body.String())
	}
}

func TestScanCacheAndStart(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default().WithDir(dir)
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	db, err := registry.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := &core.Service{Cfg: cfg, Tmux: tmux.NewFake(), DB: db, Sampler: monitor.NewSampler(nil)}
	srv := NewWith(cfg, svc, nil, Deps{Scan: scan.NewRunner(cfg)})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/scan", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("cache %d %s", rec.Code, rec.Body.String())
	}

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644)
	body, _ := json.Marshal(map[string]any{"paths": []string{root}, "refresh": true, "all": false})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/scan", bytes.NewReader(body))
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("start %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.ID == "" {
		t.Fatalf("%s", rec.Body.String())
	}
}
