package gcp_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/gcp"
	"github.com/rushikesh/devhub/internal/registry"
	"github.com/rushikesh/devhub/internal/scrub"
	"github.com/rushikesh/devhub/internal/secrets"
)

func TestSecretNeverPersisted(t *testing.T) {
	secret := "leak-me-not-12345"
	dir := t.TempDir()
	db, err := registry.Open(filepath.Join(dir, "devhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cfg := config.Default().WithDir(dir)
	cfg.GCP.Projects = []config.GCPProject{{ID: "d", Alias: "dev", Allow: []string{"logs.read", "secrets.read"}}}
	g := gcp.NewGateway(cfg, db, nil)
	g.Factory = func(ctx context.Context, p config.GCPProject) (*gcp.Runtime, error) {
		return &gcp.Runtime{
			ProjectID: p.ID,
			Logs:      logsStub{[]gcp.LogEntry{{Text: "token=" + secret}}},
			Secrets:   secretStub{secret},
		}, nil
	}
	b := secrets.New(g)
	b.KC = mem{m: map[string]string{"k": secret}}
	_, _, err = b.Resolve(context.Background(), "keychain://k")
	if err != nil {
		t.Fatal(err)
	}
	out, err := g.Call(context.Background(), "dev", "logs.read", json.RawMessage(`{}`), gcp.CallOpts{Caller: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), secret) {
		t.Fatal("API response leaked secret")
	}
	if strings.Contains(scrub.Scrub("x="+secret), secret) {
		t.Fatal("scrub missed")
	}
	rows, _ := db.ListAudit(20)
	for _, r := range rows {
		if strings.Contains(r.Result, secret) || strings.Contains(r.ParamsHash, secret) {
			t.Fatal("audit leaked")
		}
	}
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(path)
		if strings.Contains(string(b), secret) {
			t.Errorf("secret in file %s", path)
		}
		return nil
	})
}

type logsStub struct{ e []gcp.LogEntry }

func (l logsStub) ListEntries(context.Context, string, string, int64) ([]gcp.LogEntry, error) {
	return l.e, nil
}

type secretStub struct{ v string }

func (s secretStub) Access(context.Context, string, string, string) (string, error) { return s.v, nil }

type mem struct{ m map[string]string }

func (k mem) Get(a string) (string, error) { return k.m[a], nil }
func (k mem) Set(a, v string) error        { k.m[a] = v; return nil }
func (k mem) Delete(a string) error        { delete(k.m, a); return nil }
func (k mem) List() ([]string, error) {
	var n []string
	for a := range k.m {
		n = append(n, a)
	}
	return n, nil
}
