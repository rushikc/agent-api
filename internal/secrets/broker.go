package secrets

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/rushikesh/devhub/internal/scrub"
)

const ServiceName = "devhub"

type GCPReader interface {
	ReadSecret(ctx context.Context, alias, secret, version string) (string, error)
}

type Ref struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Raw    string `json:"ref"`
}

type cached struct {
	value  string
	until  time.Time
	source string
}

type Broker struct {
	GCP GCPReader
	TTL time.Duration
	Now func() time.Time
	Op  func(ref string) (string, error)
	KC  Keychain

	mu    sync.Mutex
	cache map[string]cached
}

func New(gcp GCPReader) *Broker {
	return &Broker{
		GCP:   gcp,
		TTL:   15 * time.Minute,
		Now:   time.Now,
		Op:    opRead,
		KC:    DefaultKeychain(),
		cache: map[string]cached{},
	}
}

func (b *Broker) Lock() {
	b.mu.Lock()
	b.cache = map[string]cached{}
	b.mu.Unlock()
	scrub.SetExactValues(nil)
}

func (b *Broker) remember(key, value, source string) {
	ttl := b.TTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	b.mu.Lock()
	b.cache[key] = cached{value: value, until: b.Now().Add(ttl), source: source}
	vals := make([]string, 0, len(b.cache))
	for _, c := range b.cache {
		if c.value != "" {
			vals = append(vals, c.value)
		}
	}
	b.mu.Unlock()
	scrub.SetExactValues(vals)
}

func (b *Broker) getCached(key string) (cached, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	c, ok := b.cache[key]
	if !ok || b.Now().After(c.until) {
		return cached{}, false
	}
	return c, true
}

func ParseRef(s string) (scheme, rest string, ok bool) {
	s = strings.TrimSpace(s)
	i := strings.Index(s, "://")
	if i <= 0 {
		return "", s, false
	}
	return strings.ToLower(s[:i]), s[i+3:], true
}

func (b *Broker) Resolve(ctx context.Context, ref string) (string, string, error) {
	if c, ok := b.getCached(ref); ok {
		return c.value, c.source, nil
	}
	scheme, rest, isRef := ParseRef(ref)
	if !isRef {
		return "", "", fmt.Errorf("not a secret reference: %s", ref)
	}
	var val, src string
	var err error
	switch scheme {
	case "keychain":
		val, err = b.KC.Get(rest)
		src = "keychain"
	case "gcp":
		val, err = b.resolveGCP(ctx, rest)
		src = "gcp"
	case "op":
		if b.Op == nil {
			return "", "", fmt.Errorf("1Password CLI not available")
		}
		val, err = b.Op("op://" + rest)
		src = "op"
	default:
		return "", "", fmt.Errorf("unknown secret scheme %s", scheme)
	}
	if err != nil {
		return "", "", err
	}
	b.remember(ref, val, src)
	return val, src, nil
}

func (b *Broker) resolveGCP(ctx context.Context, rest string) (string, error) {
	if b.GCP == nil {
		return "", fmt.Errorf("gcp secrets not configured")
	}
	alias, secret, ok := strings.Cut(rest, "/")
	if !ok || alias == "" || secret == "" {
		return "", fmt.Errorf("gcp ref must be gcp://<alias>/<secret>[#version]")
	}
	version := ""
	if s, v, ok := strings.Cut(secret, "#"); ok {
		secret, version = s, v
	}
	return b.GCP.ReadSecret(ctx, alias, secret, version)
}

func (b *Broker) ResolveMap(ctx context.Context, env map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range env {
		if _, _, ok := ParseRef(v); ok {
			val, _, err := b.Resolve(ctx, v)
			if err == nil {
				out[k] = val
				continue
			}
		}
		out[k] = v
	}
	return out
}

func (b *Broker) Check(ctx context.Context, refs []string) []map[string]any {
	var out []map[string]any
	for _, r := range refs {
		item := map[string]any{"ref": r, "ok": false}
		if _, src, err := b.Resolve(ctx, r); err == nil {
			item["ok"] = true
			item["source"] = src
		} else {
			item["error"] = err.Error()
		}
		out = append(out, item)
	}
	return out
}

func (b *Broker) ListKeychain() ([]Ref, error) {
	names, err := b.KC.List()
	if err != nil {
		return nil, err
	}
	var out []Ref
	for _, n := range names {
		out = append(out, Ref{Name: n, Source: "keychain", Raw: "keychain://" + n})
	}
	return out, nil
}

func (b *Broker) Set(name, value string) error {
	return b.KC.Set(name, value)
}

func (b *Broker) Remove(name string) error {
	return b.KC.Delete(name)
}

func opRead(ref string) (string, error) {
	if _, err := exec.LookPath("op"); err != nil {
		return "", fmt.Errorf("op CLI not installed")
	}
	cmd := exec.Command("op", "read", ref)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("op read: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func OpInstalled() bool {
	_, err := exec.LookPath("op")
	return err == nil
}
