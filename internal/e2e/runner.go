package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/events"
	"github.com/rushikesh/devhub/internal/profile"
	"github.com/rushikesh/devhub/internal/registry"
	"github.com/rushikesh/devhub/internal/secrets"
)

type RunRequest struct {
	Project string            `json:"project"`
	Grep    string            `json:"grep,omitempty"`
	Shard   string            `json:"shard,omitempty"`
	Force   bool              `json:"force_build,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type Queue struct {
	Cfg     *config.Config
	DB      *registry.DB
	Eng     Engine
	Bus     *events.Bus
	Secrets *secrets.Broker
	Scrub   func(string) string
	Conc    int

	mu       sync.Mutex
	inflight int
	wait     []*job
	cancels  map[string]context.CancelFunc
	logs     map[string]*bytes.Buffer
}

type job struct {
	run registry.E2ERun
	req RunRequest
	pr  *profile.Profile
}

func NewQueue(cfg *config.Config, db *registry.DB, eng Engine, bus *events.Bus, sec *secrets.Broker) *Queue {
	n := 2
	if cfg != nil && cfg.E2E.Concurrency > 0 {
		n = cfg.E2E.Concurrency
	}
	return &Queue{Cfg: cfg, DB: db, Eng: eng, Bus: bus, Secrets: sec, Conc: n, cancels: map[string]context.CancelFunc{}, logs: map[string]*bytes.Buffer{}}
}

func (q *Queue) requireEngine() error {
	if q == nil || q.Eng == nil {
		return ErrEngineDown
	}
	if err := q.Eng.Ping(); err != nil {
		return ErrEngineDown
	}
	return nil
}

func (q *Queue) failEvent(run registry.E2ERun, err error) {
	if q.Bus == nil {
		return
	}
	q.Bus.Publish(events.Event{Type: events.E2EFailed, Target: run.ID, Project: run.Project, Payload: map[string]any{"status": "failed", "error": err.Error()}})
}

func (q *Queue) Enqueue(req RunRequest, pr *profile.Profile) (registry.E2ERun, error) {
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	run := registry.E2ERun{
		ID: id, Project: req.Project, Status: "queued",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := q.requireEngine(); err != nil {
		run.Status = "failed"
		run.Summary = err.Error()
		_ = q.DB.InsertE2ERun(run)
		q.failEvent(run, err)
		return run, err
	}
	if err := q.DB.InsertE2ERun(run); err != nil {
		return run, err
	}
	q.mu.Lock()
	q.wait = append(q.wait, &job{run: run, req: req, pr: pr})
	q.mu.Unlock()
	q.kick()
	return run, nil
}

func (q *Queue) kick() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for q.inflight < q.Conc && len(q.wait) > 0 {
		j := q.wait[0]
		q.wait = q.wait[1:]
		q.inflight++
		ctx, cancel := context.WithCancel(context.Background())
		q.cancels[j.run.ID] = cancel
		go func(j *job, ctx context.Context) {
			defer func() {
				q.mu.Lock()
				q.inflight--
				delete(q.cancels, j.run.ID)
				q.mu.Unlock()
				q.kick()
			}()
			q.execute(ctx, j)
		}(j, ctx)
	}
}

func (q *Queue) Cancel(id string) error {
	q.mu.Lock()
	if c, ok := q.cancels[id]; ok {
		c()
		q.mu.Unlock()
		return nil
	}
	var kept []*job
	found := false
	for _, j := range q.wait {
		if j.run.ID == id {
			found = true
			j.run.Status = "cancelled"
			_ = q.DB.UpdateE2ERun(j.run)
			continue
		}
		kept = append(kept, j)
	}
	q.wait = kept
	q.mu.Unlock()
	if !found {
		return fmt.Errorf("run %s not cancellable", id)
	}
	return nil
}

func (q *Queue) Logs(id string) string {
	q.mu.Lock()
	defer q.mu.Unlock()
	if b := q.logs[id]; b != nil {
		return b.String()
	}
	if q.Cfg == nil {
		return ""
	}
	p := filepath.Join(q.Cfg.E2EDir(), "*", id, "run.log")
	matches, _ := filepath.Glob(p)
	if len(matches) == 0 {
		return ""
	}
	b, _ := os.ReadFile(matches[0])
	return string(b)
}

func (q *Queue) appendLog(id, s string) {
	q.mu.Lock()
	if q.logs[id] == nil {
		q.logs[id] = &bytes.Buffer{}
	}
	q.logs[id].WriteString(s)
	q.mu.Unlock()
}

func (q *Queue) execute(ctx context.Context, j *job) {
	run := j.run
	run.Status = "running"
	_ = q.DB.UpdateE2ERun(run)
	start := time.Now()
	err := q.runJob(ctx, j, &run)
	run.DurationMS = time.Since(start).Milliseconds()
	if ctx.Err() != nil {
		run.Status = "cancelled"
	} else if err != nil {
		run.Status = "failed"
		if run.Summary == "" {
			run.Summary = err.Error()
		}
	} else if run.Status == "running" {
		run.Status = "passed"
	}
	_ = q.DB.UpdateE2ERun(run)
	if q.Bus != nil {
		kind := events.E2EFinished
		if run.Status == "failed" {
			kind = events.E2EFailed
		}
		q.Bus.Publish(events.Event{Type: kind, Target: run.ID, Project: run.Project, Payload: map[string]any{"status": run.Status, "artifact": run.ArtifactPath}})
	}
}

func (q *Queue) runJob(ctx context.Context, j *job, run *registry.E2ERun) (err error) {
	pr := j.pr
	cfg := pr.E2E
	dir := pr.Path
	netName := "dh-e2e-" + run.ID
	var created []string
	netID := ""
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic: %v", rec)
		}
		for i := len(created) - 1; i >= 0; i-- {
			_ = q.Eng.RemoveContainer(context.Background(), created[i])
		}
		if netID != "" {
			_ = q.Eng.RemoveNetwork(context.Background(), netID)
		}
	}()

	df := cfg.Dockerfile
	if df == "" {
		df = "e2e/Dockerfile"
	}
	dfPath := df
	if !filepath.IsAbs(dfPath) {
		dfPath = filepath.Join(dir, df)
	}
	locks := lockfiles(dir)
	tag, err := ImageTag(cfg.Image, dfPath, locks)
	if err != nil {
		return err
	}
	if j.req.Force || !q.Eng.HasImage(ctx, tag) {
		q.appendLog(run.ID, "building "+tag+"\n")
		if err := q.Eng.BuildImage(ctx, filepath.Dir(dfPath), filepath.Base(dfPath), tag, logWriter{q, run.ID}); err != nil {
			return err
		}
	}

	snap, err := SnapshotRepo(dir)
	if err != nil {
		return err
	}
	run.GitSHA = snap.SHA
	run.Dirty = snap.Dirty

	timeout := 20 * time.Minute
	if cfg.Timeout != "" {
		if d, e := time.ParseDuration(cfg.Timeout); e == nil {
			timeout = d
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	netID, err = q.Eng.CreateNetwork(ctx, netName)
	if err != nil {
		return err
	}

	env := map[string]string{}
	for k, v := range cfg.Env {
		env[k] = v
	}
	for k, v := range j.req.Env {
		env[k] = v
	}
	if q.Secrets != nil {
		for _, name := range cfg.Secrets {
			ref := name
			if v, ok := env[name]; ok {
				ref = v
			} else if pr.Env != nil {
				if v, ok := pr.Env[name]; ok {
					ref = v
				} else {
					ref = "keychain://" + name
				}
			} else {
				ref = "keychain://" + name
			}
			if _, _, ok := secrets.ParseRef(ref); ok {
				val, _, e := q.Secrets.Resolve(ctx, ref)
				if e != nil {
					return fmt.Errorf("secret %s: %w", name, e)
				}
				env[name] = val
			}
		}
		env = q.Secrets.ResolveMap(ctx, env)
	}

	for _, svc := range cfg.Services {
		id, e := q.Eng.CreateContainer(ctx, ContainerSpec{
			Name: netName + "-" + svc.Name, Image: svc.Image, Env: svc.Env,
			Network: netID, Hostname: svc.Name,
		})
		if e != nil {
			return e
		}
		created = append(created, id)
		if e := q.Eng.StartContainer(ctx, id); e != nil {
			return e
		}
		if svc.Ready.TCP > 0 {
			ip, _ := q.Eng.ContainerIP(ctx, id)
			if e := waitTCP(ctx, ip, svc.Ready.TCP); e != nil {
				return e
			}
		}
	}

	for _, vol := range cfg.CacheVolumes {
		name := cacheVolName(pr.Name, vol)
		_ = q.Eng.EnsureVolume(ctx, name)
	}
	var binds []string
	for _, vol := range cfg.CacheVolumes {
		dest := vol
		if strings.HasPrefix(vol, "~") {
			dest = "/cache/" + filepath.Base(vol)
		} else if !strings.HasPrefix(vol, "/") {
			dest = "/work/" + vol
		}
		binds = append(binds, cacheVolName(pr.Name, vol)+":"+dest)
	}

	cmd := strings.Fields(cfg.Command)
	if len(cmd) == 0 {
		cmd = []string{"npx", "playwright", "test"}
	}
	if j.req.Grep != "" {
		cmd = append(cmd, "--grep", j.req.Grep)
	}
	if j.req.Shard != "" {
		cmd = append(cmd, "--shard", j.req.Shard)
	}

	cpus := cfg.Resources.CPUs
	mem := parseMem(cfg.Resources.Memory)
	jobID, err := q.Eng.CreateContainer(ctx, ContainerSpec{
		Name: netName + "-job", Image: tag, Cmd: cmd, Env: env,
		Network: netID, Hostname: "e2e", Binds: binds, CPUs: cpus, Memory: mem, WorkDir: "/work",
	})
	if err != nil {
		return err
	}
	created = append(created, jobID)
	if err := q.Eng.CopyTo(ctx, jobID, "/work", bytes.NewReader(snap.Tar)); err != nil {
		return err
	}
	if err := q.Eng.StartContainer(ctx, jobID); err != nil {
		return err
	}

	if rc, e := q.Eng.Logs(ctx, jobID); e == nil {
		defer rc.Close()
		go func() { _, _ = io.Copy(logWriter{q, run.ID}, rc) }()
	}
	code, err := q.Eng.WaitContainer(ctx, jobID)
	run.ExitCode = &code
	if err != nil {
		return err
	}

	artDir := filepath.Join(q.Cfg.E2EDir(), pr.Name, run.ID)
	_ = os.MkdirAll(artDir, 0o755)
	run.ArtifactPath = artDir
	q.mu.Lock()
	if b := q.logs[run.ID]; b != nil {
		body := b.String()
		if q.Scrub != nil {
			body = q.Scrub(body)
		}
		_ = os.WriteFile(filepath.Join(artDir, "run.log"), []byte(body), 0o644)
	}
	q.mu.Unlock()
	for _, a := range cfg.Artifacts {
		src := a
		if !strings.HasPrefix(src, "/") {
			src = "/work/" + a
		}
		rc, e := q.Eng.CopyFrom(ctx, jobID, src)
		if e != nil {
			continue
		}
		dest := filepath.Join(artDir, filepath.Base(a)+".tar")
		f, e := os.Create(dest)
		if e == nil {
			_, _ = io.Copy(f, rc)
			_ = f.Close()
		}
		_ = rc.Close()
	}
	if jpath := FindJUnit(artDir); jpath != "" {
		if sum, e := ParseJUnit(jpath); e == nil {
			raw, _ := json.Marshal(sum)
			run.Summary = string(raw)
			if sum.Failed > 0 {
				run.Status = "failed"
			} else {
				run.Status = "passed"
			}
		}
	} else if code != 0 {
		run.Status = "failed"
	} else {
		run.Status = "passed"
	}
	return nil
}

type logWriter struct {
	q  *Queue
	id string
}

func (w logWriter) Write(p []byte) (int, error) {
	w.q.appendLog(w.id, string(p))
	return len(p), nil
}

func waitTCP(ctx context.Context, ip string, port int) error {
	addr := net.JoinHostPort(ip, strconv.Itoa(port))
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for {
		d := net.Dialer{Timeout: 200 * time.Millisecond}
		c, err := d.DialContext(ctx, "tcp", addr)
		if err == nil {
			_ = c.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("ready tcp %s: %w", addr, ctx.Err())
		case <-t.C:
		}
	}
}

func cacheVolName(project, vol string) string {
	safe := strings.Map(func(r rune) rune {
		if r == '/' || r == ':' {
			return '-'
		}
		return r
	}, vol)
	return "dh-e2e-" + project + "-" + strings.TrimPrefix(safe, "-")
}

func lockfiles(dir string) []string {
	var out []string
	for _, n := range []string{"package-lock.json", "pnpm-lock.yaml", "yarn.lock", "bun.lock", "bun.lockb", "go.sum"} {
		p := filepath.Join(dir, n)
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func parseMem(s string) int64 {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0
	}
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "g"):
		mult = 1 << 30
		s = strings.TrimSuffix(s, "g")
	case strings.HasSuffix(s, "m"):
		mult = 1 << 20
		s = strings.TrimSuffix(s, "m")
	}
	n, _ := strconv.ParseFloat(s, 64)
	return int64(n * float64(mult))
}

func DetectRuntime() string {
	host := os.Getenv("DOCKER_HOST")
	switch {
	case strings.Contains(host, "orbstack"):
		return "OrbStack"
	case strings.Contains(host, "colima"):
		return "Colima"
	case strings.Contains(host, "docker.sock") && fileHas("/Users/"+os.Getenv("USER")+"/.orbstack"):
		return "OrbStack"
	}
	home, _ := os.UserHomeDir()
	if _, err := os.Stat(filepath.Join(home, ".orbstack")); err == nil {
		return "OrbStack"
	}
	if _, err := os.Stat(filepath.Join(home, ".colima")); err == nil {
		return "Colima"
	}
	if host != "" {
		return "Docker (" + host + ")"
	}
	return "Docker Desktop"
}

func fileHas(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
