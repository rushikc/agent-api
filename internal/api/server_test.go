package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/monitor"
	"github.com/rushikesh/devhub/internal/registry"
	"github.com/rushikesh/devhub/internal/tmux"
)

func TestHealthAndCreateUnix(t *testing.T) {
	dir := t.TempDir()
	db, err := registry.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cfg := config.Default()
	proj := t.TempDir()
	cfg.Projects = map[string]string{"krayon": proj}
	svc := &core.Service{Cfg: cfg, Tmux: tmux.NewFake(), DB: db, Sampler: monitor.NewSampler(nil)}
	srv := New(cfg, svc, nil)
	sock := filepath.Join(dir, "devhub.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := ServeUnix(ctx, srv, sock); err != nil {
			t.Log("serve:", err)
		}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	hc := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", sock)
		},
	}}
	resp, err := hc.Get("http://devhub/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal(resp.Status)
	}
	body, _ := json.Marshal(map[string]any{"project": "krayon", "kind": "active", "name": "v1"})
	resp, err = hc.Post("http://devhub/v1/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("create %s", resp.Status)
	}
}

func TestReloadUpdatesProjects(t *testing.T) {
	dir := t.TempDir()
	proj := t.TempDir()
	onDisk := config.Default().WithDir(dir)
	onDisk.Projects = map[string]string{"demo": proj}
	if err := onDisk.Save(); err != nil {
		t.Fatal(err)
	}
	live := config.Default().WithDir(dir)
	db, err := registry.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := &core.Service{Cfg: live, Tmux: tmux.NewFake(), DB: db, Sampler: monitor.NewSampler(nil)}
	srv := New(live, svc, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/reload", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if _, err := live.ProjectPath("demo"); err != nil {
		t.Fatal(err)
	}
}
