package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/e2e"
	"github.com/rushikesh/devhub/internal/events"
	"github.com/rushikesh/devhub/internal/gcp"
	"github.com/rushikesh/devhub/internal/profile"
	"github.com/rushikesh/devhub/internal/scrub"
	"github.com/rushikesh/devhub/internal/secrets"
)

func (s *Server) registerV3() {
	s.Register("GET", "/v1/gcp/projects", s.gcpProjects)
	s.Register("POST", "/v1/gcp/{alias}/{task}", s.gcpCall)
	s.Register("GET", "/v1/gcp/auth", s.gcpAuth)
	s.Register("POST", "/v1/e2e/{project}/runs", s.e2eStart)
	s.Register("GET", "/v1/e2e/runs", s.e2eList)
	s.Register("GET", "/v1/e2e/runs/{id}", s.e2eGet)
	s.Register("GET", "/v1/e2e/runs/{id}/logs", s.e2eLogs)
	s.Register("GET", "/v1/e2e/runs/{id}/logs/stream", s.e2eLogsStream)
	s.Register("DELETE", "/v1/e2e/runs/{id}", s.e2eCancel)
	s.Register("POST", "/v1/e2e/prune", s.e2ePrune)
	s.Register("POST", "/v1/e2e/{project}/build", s.e2eBuild)
	s.Register("GET", "/v1/secrets", s.secretsList)
	s.Register("POST", "/v1/secrets", s.secretsSet)
	s.Register("DELETE", "/v1/secrets/{name}", s.secretsRm)
	s.Register("POST", "/v1/secrets/check", s.secretsCheck)
	s.Register("POST", "/v1/secrets/lock", s.secretsLock)
	s.Register("GET", "/v1/alerts", s.alertsList)
	s.Register("POST", "/v1/alerts/test", s.alertsTest)
	s.Register("POST", "/v1/alerts/mute", s.alertsMute)
	s.Register("GET", "/v1/events", s.eventsRecent)
}

func (s *Server) overlay(r *http.Request) []config.GCPProject {
	name := r.URL.Query().Get("project")
	if name == "" {
		return nil
	}
	pr, err := profile.Find(s.Cfg, name)
	if err != nil {
		return nil
	}
	return pr.GCP.Projects
}

func (s *Server) gcpProjects(w http.ResponseWriter, r *http.Request) {
	if s.GCP == nil {
		http.Error(w, "gcp unavailable", 503)
		return
	}
	var out []any
	for _, p := range s.GCP.Projects(s.overlay(r)) {
		out = append(out, gcp.ProjectSummary(p))
	}
	writeJSON(w, out)
}

func (s *Server) gcpAuth(w http.ResponseWriter, r *http.Request) {
	if s.GCP == nil || s.GCP.Auth == nil {
		http.Error(w, "gcp unavailable", 503)
		return
	}
	info, err := s.GCP.Auth.Check(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	for _, p := range s.GCP.Projects(s.overlay(r)) {
		if s.GCP.Auth.Reachable(r.Context(), p.ID) {
			info.Projects = append(info.Projects, p.Alias)
		}
	}
	writeJSON(w, info)
}

func (s *Server) gcpCall(w http.ResponseWriter, r *http.Request) {
	if s.GCP == nil {
		http.Error(w, "gcp unavailable", 503)
		return
	}
	alias := r.PathValue("alias")
	task := r.PathValue("task")
	body, _ := io.ReadAll(r.Body)
	if len(body) == 0 {
		body = []byte("{}")
	}
	viaMCP := caller(r) == "mcp"
	confirmed := r.URL.Query().Get("confirm") == "1" || r.Header.Get("X-Devhub-Confirm") == "1"
	out, err := s.GCP.Call(r.Context(), alias, task, json.RawMessage(body), gcp.CallOpts{
		Caller: caller(r), Confirmed: confirmed, ViaMCP: viaMCP, Overlay: s.overlay(r),
	})
	if err != nil {
		code := 400
		if strings.Contains(err.Error(), "rejects MCP") || strings.Contains(err.Error(), "not allowlisted") {
			code = 403
		}
		http.Error(w, err.Error(), code)
		return
	}
	writeJSON(w, out)
}

func (s *Server) e2eStart(w http.ResponseWriter, r *http.Request) {
	if s.E2E == nil {
		http.Error(w, "e2e unavailable", 503)
		return
	}
	name := r.PathValue("project")
	pr, err := profile.Find(s.Cfg, name)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	var req e2e.RunRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	req.Project = pr.Name
	run, err := s.E2E.Enqueue(req, pr)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, run)
}

func (s *Server) e2eList(w http.ResponseWriter, r *http.Request) {
	if s.Core == nil {
		http.Error(w, "unavailable", 503)
		return
	}
	rows, err := s.Core.DB.ListE2ERuns(r.URL.Query().Get("project"), 50)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, rows)
}

func (s *Server) e2eGet(w http.ResponseWriter, r *http.Request) {
	run, err := s.Core.DB.GetE2ERun(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	writeJSON(w, run)
}

func (s *Server) e2eLogs(w http.ResponseWriter, r *http.Request) {
	if s.E2E == nil {
		http.Error(w, "e2e unavailable", 503)
		return
	}
	writeJSON(w, map[string]string{"logs": scrub.Scrub(s.E2E.Logs(r.PathValue("id")))})
}

func (s *Server) e2eLogsStream(w http.ResponseWriter, r *http.Request) {
	if s.E2E == nil {
		http.Error(w, "e2e unavailable", 503)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no flush", 500)
		return
	}
	id := r.PathValue("id")
	w.Header().Set("Content-Type", "text/event-stream")
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			out := scrub.Scrub(s.E2E.Logs(id))
			b, _ := json.Marshal(out)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
			run, err := s.Core.DB.GetE2ERun(id)
			if err == nil && run.Status != "queued" && run.Status != "running" {
				return
			}
		}
	}
}

func (s *Server) e2eCancel(w http.ResponseWriter, r *http.Request) {
	if s.E2E == nil {
		http.Error(w, "e2e unavailable", 503)
		return
	}
	if err := s.E2E.Cancel(r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) e2ePrune(w http.ResponseWriter, r *http.Request) {
	if s.E2E == nil {
		http.Error(w, "e2e unavailable", 503)
		return
	}
	if err := s.E2E.Prune(r.Context()); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) e2eBuild(w http.ResponseWriter, r *http.Request) {
	if s.E2E == nil {
		http.Error(w, "e2e unavailable", 503)
		return
	}
	pr, err := profile.Find(s.Cfg, r.PathValue("project"))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	tag, err := s.E2E.Build(r.Context(), pr.Path, pr.E2E.Dockerfile, pr.E2E.Image)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]string{"tag": tag})
}

func (s *Server) secretsList(w http.ResponseWriter, r *http.Request) {
	if s.Secrets == nil {
		writeJSON(w, []any{})
		return
	}
	refs, err := s.Secrets.ListKeychain()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var out []map[string]any
	for _, ref := range refs {
		item := map[string]any{"name": ref.Name, "source": ref.Source, "ref": ref.Raw, "ok": false}
		if _, _, err := s.Secrets.Resolve(r.Context(), ref.Raw); err == nil {
			item["ok"] = true
		}
		out = append(out, item)
	}
	writeJSON(w, out)
}

func (s *Server) secretsSet(w http.ResponseWriter, r *http.Request) {
	if s.Secrets == nil {
		http.Error(w, "secrets unavailable", 503)
		return
	}
	var body struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := s.Secrets.Set(body.Name, body.Value); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) secretsRm(w http.ResponseWriter, r *http.Request) {
	if s.Secrets == nil {
		http.Error(w, "secrets unavailable", 503)
		return
	}
	if err := s.Secrets.Remove(r.PathValue("name")); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) secretsCheck(w http.ResponseWriter, r *http.Request) {
	if s.Secrets == nil {
		http.Error(w, "secrets unavailable", 503)
		return
	}
	var body struct {
		Project string   `json:"project"`
		Refs    []string `json:"refs"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Project != "" {
		pr, err := profile.Find(s.Cfg, body.Project)
		if err == nil {
			for k, v := range pr.Env {
				if _, _, ok := secrets.ParseRef(v); ok {
					body.Refs = append(body.Refs, k+"="+v)
				}
			}
			for _, n := range pr.E2E.Secrets {
				if v, ok := pr.Env[n]; ok {
					body.Refs = append(body.Refs, n+"="+v)
				} else {
					body.Refs = append(body.Refs, n+"=keychain://"+n)
				}
			}
		}
	}
	var checks []string
	for _, rref := range body.Refs {
		if _, rest, ok := strings.Cut(rref, "="); ok {
			checks = append(checks, rest)
		} else {
			checks = append(checks, rref)
		}
	}
	writeJSON(w, s.Secrets.Check(r.Context(), checks))
}

func (s *Server) secretsLock(w http.ResponseWriter, r *http.Request) {
	if s.Secrets != nil {
		s.Secrets.Lock()
	}
	w.WriteHeader(204)
}

func (s *Server) alertsList(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, err := s.Core.DB.ListEvents(n)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, rows)
}

func (s *Server) alertsTest(w http.ResponseWriter, r *http.Request) {
	if s.Alerts == nil {
		http.Error(w, "alerts unavailable", 503)
		return
	}
	s.Alerts.Handle(events.Event{Type: "alerts.test", Target: "test"})
	w.WriteHeader(204)
}

func (s *Server) alertsMute(w http.ResponseWriter, r *http.Request) {
	if s.Alerts == nil {
		http.Error(w, "alerts unavailable", 503)
		return
	}
	var body struct {
		Duration string `json:"duration"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	d, err := time.ParseDuration(body.Duration)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.Alerts.Mute(d)
	w.WriteHeader(204)
}

func (s *Server) eventsRecent(w http.ResponseWriter, r *http.Request) {
	s.alertsList(w, r)
}
