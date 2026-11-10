package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/jsonx"
	"github.com/rushikesh/devhub/internal/monitor"
	"github.com/rushikesh/devhub/internal/registry"
	"github.com/rushikesh/devhub/internal/tmux"
)

func TestListEndpointsEmptyArray(t *testing.T) {
	dir := t.TempDir()
	db, err := registry.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cfg := config.Default().WithDir(dir)
	svc := &core.Service{Cfg: cfg, Tmux: tmux.NewFake(), DB: db, Sampler: monitor.NewSampler(nil)}
	srv := New(cfg, svc, nil)
	paths := []string{
		"/v1/sessions",
		"/v1/skills",
		"/v1/ports",
		"/v1/orphans",
		"/v1/profiles",
		"/v1/e2e/runs",
		"/v1/secrets",
		"/v1/alerts",
		"/v1/events",
	}
	for _, p := range paths {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, nil)
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s status %d %s", p, rec.Code, rec.Body.String())
		}
		var got any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("%s json %v %s", p, err, rec.Body.String())
		}
		if _, ok := got.([]any); !ok {
			t.Fatalf("%s want array got %T %s", p, got, rec.Body.String())
		}
	}
	b, _ := jsonx.Compact([]string(nil))
	if string(b) != "[]" {
		t.Fatal(string(b))
	}
}
