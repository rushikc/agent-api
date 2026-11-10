package gcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/registry"
	"github.com/rushikesh/devhub/internal/scrub"
)

type fakeLogs struct{ entries []LogEntry }

func (f fakeLogs) ListEntries(context.Context, string, string, int64) ([]LogEntry, error) {
	return f.entries, nil
}

type fakeBQ struct{}

func (fakeBQ) DryRun(context.Context, string, string, int64) (int64, error) { return 1, nil }
func (fakeBQ) Query(context.Context, string, string, int64, int) ([]BQRow, error) {
	return []BQRow{{"n": 1}}, nil
}

func testGW(t *testing.T, projects []config.GCPProject) *Gateway {
	t.Helper()
	db, err := registry.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	g := NewGateway(&config.Config{GCP: config.GCPConfig{Projects: projects, RateLimitPerMin: 30}}, db, nil)
	g.Factory = func(ctx context.Context, p config.GCPProject) (*Runtime, error) {
		return &Runtime{
			ProjectID: p.ID, DryFirst: p.BQ.DryRunFirst, BQMax: ParseBytes(p.BQ.MaxBytesBilled),
			Logs: fakeLogs{entries: []LogEntry{{Severity: "ERROR", Text: "boom"}}},
			BQ:   fakeBQ{},
		}, nil
	}
	return g
}

func TestAllowlistAndProdMCP(t *testing.T) {
	g := testGW(t, []config.GCPProject{
		{ID: "dev-id", Alias: "dev", Allow: []string{"logs.read", "bq.query"}},
		{ID: "prod-id", Alias: "prod", Allow: []string{"logs.read"}, RequireConfirm: true},
	})
	ctx := context.Background()
	_, err := g.Call(ctx, "prod", "bq.query", mustJSON(bqParams{SQL: "SELECT 1"}), CallOpts{Caller: "mcp", ViaMCP: true})
	if err == nil || !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("allowlist: %v", err)
	}
	_, err = g.Call(ctx, "prod", "logs.read", mustJSON(logsParams{Limit: 10}), CallOpts{Caller: "mcp", ViaMCP: true})
	if err == nil || !strings.Contains(err.Error(), "rejects MCP") {
		t.Fatalf("prod mcp: %v", err)
	}
	g.AllowMCPProd = true
	out, err := g.Call(ctx, "dev", "logs.read", mustJSON(logsParams{Limit: 10}), CallOpts{Caller: "mcp", ViaMCP: true})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Fatal("nil logs")
	}
}

func TestBQRejectsDML(t *testing.T) {
	if err := ValidateSelect("DELETE FROM t"); err == nil {
		t.Fatal("delete")
	}
	if err := ValidateSelect("SELECT * FROM t"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSelect("INSERT INTO t SELECT 1"); err == nil {
		t.Fatal("insert")
	}
	g := testGW(t, []config.GCPProject{{ID: "d", Alias: "dev", Allow: []string{"bq.query"}, BQ: config.BQPolicy{DryRunFirst: true}}})
	_, err := g.Call(context.Background(), "dev", "bq.query", mustJSON(bqParams{SQL: "DROP TABLE t"}), CallOpts{Caller: "cli", Confirmed: true})
	if err == nil {
		t.Fatal("expected reject")
	}
}

func TestSecretsNotViaAPI(t *testing.T) {
	g := testGW(t, []config.GCPProject{{ID: "d", Alias: "dev", Allow: []string{"secrets.read"}}})
	_, err := g.Call(context.Background(), "dev", "secrets.read", mustJSON(secretsParams{Secret: "x"}), CallOpts{Caller: "api"})
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("%v", err)
	}
}

func TestAuditHasNoSecret(t *testing.T) {
	secret := "super-secret-value-xyz"
	scrub.SetExactValues([]string{secret})
	t.Cleanup(func() { scrub.SetExactValues(nil) })
	g := testGW(t, []config.GCPProject{{ID: "d", Alias: "dev", Allow: []string{"logs.read"}}})
	g.Factory = func(ctx context.Context, p config.GCPProject) (*Runtime, error) {
		return &Runtime{ProjectID: p.ID, Logs: fakeLogs{entries: []LogEntry{{Text: "token=" + secret}}}}, nil
	}
	out, err := g.Call(context.Background(), "dev", "logs.read", json.RawMessage(`{}`), CallOpts{Caller: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(out)
	if strings.Contains(string(b), secret) {
		t.Fatal("secret leaked in response")
	}
	rows, _ := g.DB.ListAudit(10)
	for _, r := range rows {
		blob := r.Action + r.Target + r.Result + r.ParamsHash
		if strings.Contains(blob, secret) {
			t.Fatalf("secret in audit %+v", r)
		}
	}
}

func TestRegistryFakeClients(t *testing.T) {
	if _, ok := Lookup("logs.read"); !ok {
		t.Fatal("logs.read not registered")
	}
	for _, n := range []string{"bq.query", "pubsub.peek", "run.status", "secrets.read", "gcs.list"} {
		if _, ok := Lookup(n); !ok {
			t.Fatal(n)
		}
	}
}
