package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/monitor"
	"github.com/rushikesh/devhub/internal/registry"
	"github.com/rushikesh/devhub/internal/tmux"
)

func TestWriteJSONNilSliceIsArray(t *testing.T) {
	var orphans []string
	rec := httptest.NewRecorder()
	writeJSON(rec, orphans)
	var got any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err, rec.Body.String())
	}
	if _, ok := got.([]any); !ok {
		t.Fatalf("want array got %T %s", got, rec.Body.String())
	}
}

func TestWriteJSONExitCode(t *testing.T) {
	code := 7
	rec := httptest.NewRecorder()
	writeJSON(rec, []registry.Session{{Name: "x", ExitCode: &code}})
	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err, rec.Body.String())
	}
	if len(got) != 1 {
		t.Fatal(got)
	}
}

func TestWrapRecovers(t *testing.T) {
	dir := t.TempDir()
	db, err := registry.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cfg := config.Default().WithDir(dir)
	svc := &core.Service{Cfg: cfg, Tmux: tmux.NewFake(), DB: db, Sampler: monitor.NewSampler(nil)}
	srv := New(cfg, svc, nil)
	srv.Register("GET", "/v1/boom", func(http.ResponseWriter, *http.Request) { panic("boom") })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/boom", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 500 {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() == 0 {
		t.Fatal("empty reply")
	}
}
