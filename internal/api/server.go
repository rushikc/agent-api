package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/rushikesh/devhub/internal/alerts"
	"github.com/rushikesh/devhub/internal/client"
	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/e2e"
	"github.com/rushikesh/devhub/internal/events"
	"github.com/rushikesh/devhub/internal/gcp"
	"github.com/rushikesh/devhub/internal/janitor"
	"github.com/rushikesh/devhub/internal/monitor"
	"github.com/rushikesh/devhub/internal/profile"
	"github.com/rushikesh/devhub/internal/registry"
	"github.com/rushikesh/devhub/internal/scrub"
	"github.com/rushikesh/devhub/internal/secrets"
	"github.com/rushikesh/devhub/internal/skills"
)

type Route struct {
	Method  string
	Path    string
	Handler http.HandlerFunc
}

type Deps struct {
	GCP     *gcp.Gateway
	E2E     *e2e.Queue
	Secrets *secrets.Broker
	Alerts  *alerts.Engine
	Bus     *events.Bus
}

type Server struct {
	Cfg          *config.Config
	Core         *core.Service
	Skills       *skills.Hub
	Token        string
	RequireToken bool
	GCP          *gcp.Gateway
	E2E          *e2e.Queue
	Secrets      *secrets.Broker
	Alerts       *alerts.Engine
	Bus          *events.Bus
	mux          *http.ServeMux
	routes       []Route
}

func New(cfg *config.Config, svc *core.Service, hub *skills.Hub) *Server {
	return NewWith(cfg, svc, hub, Deps{})
}

func NewWith(cfg *config.Config, svc *core.Service, hub *skills.Hub, d Deps) *Server {
	if d.E2E != nil && d.E2E.Scrub == nil {
		d.E2E.Scrub = scrub.Scrub
	}
	s := &Server{Cfg: cfg, Core: svc, Skills: hub, GCP: d.GCP, E2E: d.E2E, Secrets: d.Secrets, Alerts: d.Alerts, Bus: d.Bus, mux: http.NewServeMux()}
	s.registerCore()
	s.registerV3()
	return s
}

func (s *Server) Register(method, path string, h http.HandlerFunc) {
	s.routes = append(s.routes, Route{Method: method, Path: path, Handler: h})
	s.mux.HandleFunc(method+" "+path, s.wrap(h))
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) wrap(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.RequireToken {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if s.Token == "" || got != s.Token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		h(w, r)
	}
}

func caller(r *http.Request) string {
	if c := r.Header.Get(client.CallerHeader); c != "" {
		return c
	}
	return "api"
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(nilSliceToEmpty(v))
}

func nilSliceToEmpty(v any) any {
	if v == nil {
		return []any{}
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice && rv.IsNil() {
		return reflect.MakeSlice(rv.Type(), 0, 0).Interface()
	}
	return v
}

func (s *Server) audit(r *http.Request, action, target string, err error) {
	res := "ok"
	if err != nil {
		res = err.Error()
	}
	s.Core.Audit(caller(r), action, target, res)
}

func (s *Server) registerCore() {
	s.Register("GET", "/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true})
	})
	s.Register("GET", "/v1/sessions", s.listSessions)
	s.Register("POST", "/v1/sessions", s.createSession)
	s.Register("POST", "/v1/sessions/{id}/restart", s.restartSession)
	s.Register("DELETE", "/v1/sessions/{id}", s.deleteSession)
	s.Register("GET", "/v1/sessions/{id}/logs", s.sessionLogs)
	s.Register("GET", "/v1/sessions/{id}/logs/stream", s.sessionLogsStream)
	s.Register("POST", "/v1/sessions/{id}/send", s.sessionSend)
	s.Register("GET", "/v1/stats", s.stats)
	s.Register("GET", "/v1/skills", s.listSkills)
	s.Register("GET", "/v1/skills/{name}", s.getSkill)
	s.Register("POST", "/v1/skills/sync", s.syncSkills)
	s.Register("GET", "/v1/profiles", s.listProfiles)
	s.Register("POST", "/v1/profiles/{project}/up", s.profileUp)
	s.Register("POST", "/v1/profiles/{project}/down", s.profileDown)
	s.Register("GET", "/v1/ports", s.listPorts)
	s.Register("POST", "/v1/ports/{port}/free", s.freePort)
	s.Register("GET", "/v1/orphans", s.listOrphans)
}

func (s *Server) sessID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	_ = s.Core.Reconcile()
	all, err := s.Core.DB.List(true)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	proj := r.URL.Query().Get("project")
	kind := r.URL.Query().Get("kind")
	var out []registry.Session
	for _, sess := range all {
		if proj != "" && sess.Project != proj {
			continue
		}
		if kind != "" && string(sess.Kind) != kind {
			continue
		}
		out = append(out, sess)
	}
	if out == nil {
		out = []registry.Session{}
	}
	writeJSON(w, out)
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Project string `json:"project"`
		Name    string `json:"name"`
		Kind    string `json:"kind"`
		Command string `json:"command"`
		Cwd     string `json:"cwd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var sess registry.Session
	var err error
	if body.Kind == "shadow" {
		sess, err = s.Core.NewShadowIn(body.Project, body.Name, body.Cwd, strings.Fields(body.Command), nil, 0)
	} else {
		sess, err = s.Core.NewActiveIn(body.Project, body.Name, body.Cwd, nil)
	}
	s.audit(r, "create", body.Project+"/"+body.Name, err)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, sess)
}

func (s *Server) restartSession(w http.ResponseWriter, r *http.Request) {
	id, err := s.sessID(r)
	if err != nil {
		http.Error(w, "bad id", 400)
		return
	}
	sess, err := s.Core.Get(id)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	err = s.Core.Restart(fmt.Sprintf("%s/%s", sess.Project, sess.Name))
	s.audit(r, "restart", fmt.Sprintf("%d", id), err)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	id, _ := s.sessID(r)
	sess, err := s.Core.Get(id)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	err = s.Core.Kill(fmt.Sprintf("%s/%s", sess.Project, sess.Name))
	s.audit(r, "kill", fmt.Sprintf("%d", id), err)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) sessionLogs(w http.ResponseWriter, r *http.Request) {
	id, _ := s.sessID(r)
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	if lines == 0 {
		lines = 200
	}
	out, err := s.Core.PeekLines(strconv.FormatInt(id, 10), lines)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]string{"logs": scrub.Scrub(out)})
}

func (s *Server) sessionLogsStream(w http.ResponseWriter, r *http.Request) {
	id, _ := s.sessID(r)
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no flush", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			out, err := s.Core.PeekLines(strconv.FormatInt(id, 10), 200)
			if err != nil {
				fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
				fl.Flush()
				return
			}
			b, _ := json.Marshal(scrub.Scrub(out))
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		}
	}
}

func (s *Server) sessionSend(w http.ResponseWriter, r *http.Request) {
	id, _ := s.sessID(r)
	var body struct {
		Keys  string `json:"keys"`
		Force bool   `json:"force"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	err := s.Core.Send(strconv.FormatInt(id, 10), body.Keys, body.Force)
	s.audit(r, "send", strconv.FormatInt(id, 10), err)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	snap, err := monitor.ReadSnapshot(s.Cfg.StatusPath())
	if err != nil {
		rows, _ := s.Core.List(true)
		var sessions []registry.Session
		for _, v := range rows {
			sessions = append(sessions, v.Session)
		}
		snap = monitor.BuildSnapshot(sessions, s.Core.Sampler)
	}
	writeJSON(w, snap)
}

func (s *Server) listSkills(w http.ResponseWriter, r *http.Request) {
	if s.Skills == nil {
		writeJSON(w, []any{})
		return
	}
	items, err := s.Skills.List()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, items)
}

func (s *Server) getSkill(w http.ResponseWriter, r *http.Request) {
	if s.Skills == nil {
		http.NotFound(w, r)
		return
	}
	it, err := s.Skills.Get(r.PathValue("name"))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	writeJSON(w, it)
}

func (s *Server) syncSkills(w http.ResponseWriter, r *http.Request) {
	if s.Skills == nil {
		http.Error(w, "skills unavailable", 500)
		return
	}
	m, err := s.Skills.Sync(false)
	s.audit(r, "skills.sync", "", err)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, m)
}

func (s *Server) listProfiles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, profile.List(s.Cfg))
}

func (s *Server) profileUp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	pr, err := profile.Find(s.Cfg, name)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	res, err := profile.Up(s.Core, pr, true)
	s.audit(r, "profile.up", name, err)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, res)
}

func (s *Server) profileDown(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("project")
	all := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"
	err := profile.Down(s.Core, name, all)
	s.audit(r, "profile.down", name, err)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) listPorts(w http.ResponseWriter, r *http.Request) {
	rows, _ := s.Core.DB.List(false)
	ports, err := janitor.ListPorts(rows, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, ports)
}

func (s *Server) freePort(w http.ResponseWriter, r *http.Request) {
	port, _ := strconv.Atoi(r.PathValue("port"))
	rows, _ := s.Core.DB.List(false)
	err := janitor.FreePort(port, rows, nil)
	s.audit(r, "free_port", r.PathValue("port"), err)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) listOrphans(w http.ResponseWriter, r *http.Request) {
	rows, _ := s.Core.DB.List(false)
	orph, err := janitor.ListOrphans(s.Cfg, rows, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, orph)
}

func ServeUnix(ctx context.Context, s *Server, sock string) error {
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return err
	}
	if err := os.Chmod(sock, 0o600); err != nil {
		_ = ln.Close()
		return err
	}
	hs := &http.Server{Handler: s.Handler()}
	go func() {
		<-ctx.Done()
		_ = hs.Close()
	}()
	err = hs.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func ServeTCP(ctx context.Context, s *Server, addr string) error {
	if !strings.HasPrefix(addr, "127.0.0.1:") && !strings.HasPrefix(addr, "localhost:") {
		return fmt.Errorf("TCP must bind 127.0.0.1, got %s", addr)
	}
	hs := &http.Server{Addr: addr, Handler: s.Handler()}
	go func() {
		<-ctx.Done()
		_ = hs.Close()
	}()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	err = hs.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func EnsureToken(cfg *config.Config) (string, error) {
	p := cfg.TokenPath()
	b, err := os.ReadFile(p)
	if err == nil && len(bytesTrim(b)) > 0 {
		return string(bytesTrim(b)), nil
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(raw)
	if err := os.WriteFile(p, []byte(tok), 0o600); err != nil {
		return "", err
	}
	return tok, nil
}

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
