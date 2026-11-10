package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rushikesh/devhub/internal/scan"
)

func (s *Server) registerScan() {
	s.Register("GET", "/v1/scan", s.scanCache)
	s.Register("POST", "/v1/scan", s.scanStart)
	s.Register("GET", "/v1/scan/{id}", s.scanJob)
	s.Register("GET", "/v1/scan/{id}/stream", s.scanStream)
	s.Register("GET", "/v1/projects", s.listProjects)
	s.Register("POST", "/v1/projects", s.addProject)
	s.Register("PATCH", "/v1/projects/{name}", s.renameProject)
	s.Register("DELETE", "/v1/projects/{name}", s.deleteProject)
}

func (s *Server) scanCache(w http.ResponseWriter, r *http.Request) {
	if s.Scan == nil {
		http.Error(w, "scan unavailable", 503)
		return
	}
	c, err := s.Scan.Cache()
	if err != nil {
		writeJSON(w, map[string]any{"repos": []any{}, "stale": true})
		return
	}
	stale := !scan.Fresh(c, c.Roots, c.Depth, c.Nested, c.IncludeVolumes, scan.DefaultMaxAge)
	writeJSON(w, map[string]any{
		"cache":  c,
		"age_ms": scan.Age(c).Milliseconds(),
		"stale":  stale,
	})
}

func (s *Server) scanStart(w http.ResponseWriter, r *http.Request) {
	if s.Scan == nil {
		http.Error(w, "scan unavailable", 503)
		return
	}
	var req scan.Request
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	j := s.Scan.Start(context.Background(), req)
	writeJSON(w, map[string]any{"id": j.ID, "status": j.Status})
}

func (s *Server) scanJob(w http.ResponseWriter, r *http.Request) {
	if s.Scan == nil {
		http.Error(w, "scan unavailable", 503)
		return
	}
	j := s.Scan.Snapshot(r.PathValue("id"))
	if j.Error == "unknown scan job" && j.Status == scan.JobError {
		http.Error(w, j.Error, 404)
		return
	}
	writeJSON(w, j)
}

func (s *Server) scanStream(w http.ResponseWriter, r *http.Request) {
	if s.Scan == nil {
		http.Error(w, "scan unavailable", 503)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no flush", 500)
		return
	}
	id := r.PathValue("id")
	w.Header().Set("Content-Type", "text/event-stream")
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			j := s.Scan.Snapshot(id)
			b, _ := json.Marshal(j)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
			if j.Status == scan.JobDone || j.Status == scan.JobError {
				return
			}
		}
	}
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		writeJSON(w, map[string]string{})
		return
	}
	writeJSON(w, s.Cfg.Projects)
}

func (s *Server) addProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	err := s.Core.AddProject(body.Name, body.Path)
	s.audit(r, "project.add", body.Name, err)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]string{"name": body.Name, "path": s.Cfg.Projects[body.Name]})
}

func (s *Server) renameProject(w http.ResponseWriter, r *http.Request) {
	old := r.PathValue("name")
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	err := s.Core.RenameProject(old, body.Name)
	s.audit(r, "project.mv", old, err)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	kill := r.URL.Query().Get("kill_sessions") == "1" || r.URL.Query().Get("kill_sessions") == "true"
	err := s.Core.RemoveProject(name, kill)
	s.audit(r, "project.rm", name, err)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(204)
}
