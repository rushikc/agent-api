package gcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/events"
	"github.com/rushikesh/devhub/internal/registry"
	"github.com/rushikesh/devhub/internal/scrub"
	"golang.org/x/time/rate"
)

type CallOpts struct {
	Caller    string
	Confirmed bool
	ViaMCP    bool
	Internal  bool
	Overlay   []config.GCPProject
}

type cacheEntry struct {
	until time.Time
	data  any
}

type Gateway struct {
	Cfg          config.GCPConfig
	AllowMCPProd bool
	DB           *registry.DB
	Bus          *events.Bus
	Factory      func(ctx context.Context, p config.GCPProject) (*Runtime, error)
	Auth         AuthAPI
	RateN        int

	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	cache    map[string]cacheEntry
}

func NewGateway(cfg *config.Config, db *registry.DB, bus *events.Bus) *Gateway {
	g := &Gateway{
		DB:       db,
		Bus:      bus,
		limiters: map[string]*rate.Limiter{},
		cache:    map[string]cacheEntry{},
		RateN:    30,
		Factory:  DefaultFactory,
	}
	if cfg != nil {
		g.Cfg = cfg.GCP
		g.AllowMCPProd = cfg.AllowProdViaMCP()
		if cfg.GCP.RateLimitPerMin > 0 {
			g.RateN = cfg.GCP.RateLimitPerMin
		}
		g.Auth = &adcAuth{impersonate: cfg.GCP.ImpersonateServiceAccount}
		imp := cfg.GCP.ImpersonateServiceAccount
		g.Factory = func(ctx context.Context, p config.GCPProject) (*Runtime, error) {
			if p.ImpersonateServiceAccount == "" {
				p.ImpersonateServiceAccount = imp
			}
			return DefaultFactory(ctx, p)
		}
	}
	return g
}

func (g *Gateway) Projects(overlay []config.GCPProject) []config.GCPProject {
	cfg := config.MergeGCP(g.Cfg, overlay)
	return cfg.Projects
}

func (g *Gateway) Resolve(alias string, overlay []config.GCPProject) (config.GCPProject, error) {
	for _, p := range g.Projects(overlay) {
		if p.Alias == alias || p.ID == alias {
			return p, nil
		}
	}
	return config.GCPProject{}, fmt.Errorf("unknown GCP alias %q", alias)
}

func allowed(p config.GCPProject, task string) bool {
	for _, a := range p.Allow {
		if a == task {
			return true
		}
	}
	return false
}

func (g *Gateway) Call(ctx context.Context, alias, taskName string, params json.RawMessage, opts CallOpts) (any, error) {
	start := time.Now()
	p, err := g.Resolve(alias, opts.Overlay)
	if err != nil {
		return nil, err
	}
	if taskName == "secrets.read" && !opts.Internal {
		return nil, fmt.Errorf("secrets.read is not available through the API")
	}
	if !allowed(p, taskName) {
		return nil, fmt.Errorf("task %q is not allowlisted for alias %q", taskName, alias)
	}
	if p.RequireConfirm {
		if opts.ViaMCP && !g.AllowMCPProd {
			return nil, fmt.Errorf("prod alias %q rejects MCP calls (set gcp.allow_prod_via_mcp=true)", alias)
		}
		if !opts.ViaMCP && !opts.Confirmed && !opts.Internal {
			return nil, fmt.Errorf("alias %q requires confirmation (-y)", alias)
		}
	}
	t, ok := Lookup(taskName)
	if !ok {
		return nil, fmt.Errorf("unknown task %q", taskName)
	}
	if err := t.Validate(params); err != nil {
		return nil, err
	}
	if err := g.limit(opts.Caller); err != nil {
		return nil, err
	}

	hash := paramsHash(taskName, params)
	if isGetLike(taskName) {
		if v, ok := g.cached(hash); ok {
			return v, nil
		}
	}

	rt, err := g.Factory(ctx, p)
	if err != nil {
		g.fail(alias, taskName, err)
		g.audit(opts, p, taskName, hash, start, 0, err)
		return nil, err
	}
	if rt.BQMax == 0 {
		rt.BQMax = ParseBytes(p.BQ.MaxBytesBilled)
	}
	if !rt.DryFirst {
		rt.DryFirst = p.BQ.DryRunFirst
	}

	out, err := t.Run(ctx, rt, params)
	if err != nil {
		g.fail(alias, taskName, err)
		g.audit(opts, p, taskName, hash, start, 0, err)
		return nil, err
	}
	if sv, ok := out.(secretValue); ok {
		g.audit(opts, p, taskName, hash, start, 0, nil)
		return sv, nil
	}
	scrubbed := scrubResult(out)
	b, _ := json.Marshal(scrubbed)
	if isGetLike(taskName) {
		g.storeCache(hash, scrubbed)
	}
	g.audit(opts, p, taskName, hash, start, int64(len(b)), nil)
	return scrubbed, nil
}

func (g *Gateway) ReadSecret(ctx context.Context, alias, secret, version string) (string, error) {
	raw, _ := json.Marshal(secretsParams{Secret: secret, Version: version})
	out, err := g.Call(ctx, alias, "secrets.read", raw, CallOpts{Caller: "secrets", Internal: true})
	if err != nil {
		return "", err
	}
	if sv, ok := out.(secretValue); ok {
		return sv.Value, nil
	}
	return "", fmt.Errorf("unexpected secrets.read result")
}

func (g *Gateway) limit(caller string) error {
	if caller == "" {
		caller = "cli"
	}
	g.mu.Lock()
	lim, ok := g.limiters[caller]
	if !ok {
		n := g.RateN
		if n <= 0 {
			n = 30
		}
		lim = rate.NewLimiter(rate.Every(time.Minute/time.Duration(n)), n)
		g.limiters[caller] = lim
	}
	g.mu.Unlock()
	if !lim.Allow() {
		return fmt.Errorf("rate limit exceeded for caller %s", caller)
	}
	return nil
}

func isGetLike(task string) bool {
	switch task {
	case "logs.read", "run.status", "gcs.list":
		return true
	}
	return false
}

func paramsHash(task string, params json.RawMessage) string {
	h := sha256.Sum256(append([]byte(task+"\x00"), params...))
	return hex.EncodeToString(h[:])
}

func (g *Gateway) cached(key string) (any, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	e, ok := g.cache[key]
	if !ok || time.Now().After(e.until) {
		return nil, false
	}
	return e.data, true
}

func (g *Gateway) storeCache(key string, data any) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cache[key] = cacheEntry{until: time.Now().Add(30 * time.Second), data: data}
}

func (g *Gateway) fail(alias, task string, err error) {
	if g.Bus != nil {
		g.Bus.Publish(events.Event{Type: events.GCPError, Target: alias + "/" + task, Payload: map[string]any{"error": err.Error()}})
	}
}

func (g *Gateway) audit(opts CallOpts, p config.GCPProject, task, hash string, start time.Time, size int64, err error) {
	if g.DB == nil {
		return
	}
	res := "ok"
	if err != nil {
		res = err.Error()
	}
	_ = g.DB.AuditCall(opts.Caller, "gcp."+task, p.Alias, res, p.ID, task, hash, time.Since(start).Milliseconds(), size)
}

func scrubResult(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	s := scrub.Scrub(string(b))
	var out any
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return v
	}
	return out
}

func AllowedContains(p config.GCPProject, task string) bool {
	return allowed(p, task)
}

func ProjectSummary(p config.GCPProject) map[string]any {
	return map[string]any{
		"id":              p.ID,
		"alias":           p.Alias,
		"allow":           p.Allow,
		"require_confirm": p.RequireConfirm,
	}
}

func NormalizeAlias(s string) string {
	return strings.TrimSpace(s)
}
