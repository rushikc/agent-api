package scan

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/rushikesh/devhub/internal/config"
)

type JobStatus string

const (
	JobQueued  JobStatus = "queued"
	JobRunning JobStatus = "running"
	JobDone    JobStatus = "done"
	JobError   JobStatus = "error"
)

type Job struct {
	ID       string    `json:"id"`
	Status   JobStatus `json:"status"`
	Progress Progress  `json:"progress"`
	Result   *Result   `json:"result,omitempty"`
	Error    string    `json:"error,omitempty"`
}

type Request struct {
	All            bool     `json:"all"`
	Paths          []string `json:"paths"`
	Refresh        bool     `json:"refresh"`
	Nested         bool     `json:"nested"`
	IncludeVolumes bool     `json:"include_volumes"`
	Depth          int      `json:"depth,omitempty"`
}

type Runner struct {
	Cfg *config.Config
	mu  sync.Mutex
	job map[string]*Job
}

func NewRunner(cfg *config.Config) *Runner {
	return &Runner{Cfg: cfg, job: map[string]*Job{}}
}

func (r *Runner) Cache() (*Cache, error) {
	if r == nil || r.Cfg == nil {
		return nil, fmt.Errorf("no config")
	}
	return LoadCache(r.Cfg.ScanCachePath())
}

func (r *Runner) Start(ctx context.Context, req Request) *Job {
	id := newID()
	j := &Job{ID: id, Status: JobQueued}
	r.mu.Lock()
	r.job[id] = j
	r.mu.Unlock()
	go r.run(ctx, j, req)
	return j
}

func (r *Runner) Get(id string) *Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.job[id]
}

func (r *Runner) Snapshot(id string) Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	j := r.job[id]
	if j == nil {
		return Job{ID: id, Status: JobError, Error: "unknown scan job"}
	}
	out := *j
	if j.Result != nil {
		cp := *j.Result
		out.Result = &cp
	}
	return out
}

func (r *Runner) run(ctx context.Context, j *Job, req Request) {
	if ctx == nil {
		ctx = context.Background()
	}
	r.set(j, func() { j.Status = JobRunning })
	cfg := r.Cfg
	vols := req.IncludeVolumes
	if cfg != nil && cfg.Scan.IncludeVolumes {
		vols = true
	}
	roots := ResolveRoots(cfg, req.All, req.Paths, vols)
	depth := ResolveDepth(cfg, req.All, req.Depth)
	exclude := []string{}
	projects := map[string]string{}
	if cfg != nil {
		exclude = cfg.Scan.Exclude
		projects = cfg.Projects
	}
	if !req.Refresh && cfg != nil {
		if c, err := LoadCache(cfg.ScanCachePath()); err == nil && Fresh(c, roots, depth, req.Nested, vols, DefaultMaxAge) {
			res := ResultFromCache(c)
			Annotate(res, projects)
			r.set(j, func() {
				j.Status = JobDone
				j.Result = res
				j.Progress = Progress{Dirs: res.Dirs, Repos: len(res.Repos), Denied: res.Denied, Privacy: res.PrivacyDenied}
			})
			return
		}
	}
	res, err := Walk(ctx, Options{
		Roots:          roots,
		Depth:          depth,
		Nested:         req.Nested,
		IncludeVolumes: vols,
		Exclude:        exclude,
		Projects:       projects,
		Progress: func(p Progress) {
			r.set(j, func() { j.Progress = p })
		},
	})
	if err != nil {
		r.set(j, func() {
			j.Status = JobError
			j.Error = err.Error()
		})
		return
	}
	if cfg != nil {
		_ = SaveCache(cfg.ScanCachePath(), CacheFromResult(res))
	}
	r.set(j, func() {
		j.Status = JobDone
		j.Result = res
		j.Progress = Progress{Dirs: res.Dirs, Repos: len(res.Repos), Denied: res.Denied, Privacy: res.PrivacyDenied, Elapsed: res.Duration}
	})
}

func (r *Runner) set(j *Job, fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn()
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:]) + strconvNow()
}

func strconvNow() string {
	return time.Now().UTC().Format("150405")
}
