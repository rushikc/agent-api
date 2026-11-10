package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rushikesh/devhub/internal/client"
	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/daemon"
)

type strArgs struct {
	Project string `json:"project,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Command string `json:"command,omitempty"`
	Name    string `json:"name,omitempty"`
	Grep    string `json:"grep,omitempty"`
	ID      int64  `json:"id,omitempty"`
	Lines   int    `json:"lines,omitempty"`
	Port    int    `json:"port,omitempty"`
}

func RunStdio(cfg *config.Config) error {
	_ = daemon.Ensure(cfg)
	c := client.Unix(cfg, "mcp")
	t := &Tools{C: c, Cfg: cfg}
	server := mcp.NewServer(&mcp.Implementation{Name: "devhub", Version: "3.0.0"}, nil)

	add := func(name, desc string, fn func(context.Context, *mcp.CallToolRequest, strArgs) (*mcp.CallToolResult, any, error)) {
		mcp.AddTool(server, &mcp.Tool{Name: name, Description: desc}, fn)
	}

	add("list_sessions", "List devhub sessions", func(ctx context.Context, req *mcp.CallToolRequest, args strArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.ListSessions(args.Project, args.Kind)
		return textResult(s, err)
	})
	add("start_shadow", "Start a shadow process", func(ctx context.Context, req *mcp.CallToolRequest, args strArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.StartShadow(args.Project, args.Command)
		return textResult(s, err)
	})
	add("restart_session", "Restart a session by id", func(ctx context.Context, req *mcp.CallToolRequest, args strArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.RestartSession(args.ID)
		return textResult(s, err)
	})
	add("stop_session", "Stop a session by id", func(ctx context.Context, req *mcp.CallToolRequest, args strArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.StopSession(args.ID)
		return textResult(s, err)
	})
	add("read_logs", "Read session logs (secrets scrubbed)", func(ctx context.Context, req *mcp.CallToolRequest, args strArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.ReadLogs(args.ID, args.Lines, args.Grep)
		return textResult(s, err)
	})
	add("get_stats", "Resource snapshot", func(ctx context.Context, req *mcp.CallToolRequest, args strArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.GetStats()
		return textResult(s, err)
	})
	add("list_ports", "Listening TCP ports", func(ctx context.Context, req *mcp.CallToolRequest, args strArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.ListPorts()
		return textResult(s, err)
	})
	add("free_port", "Free a TCP port (requires mcp.allow_port_kill)", func(ctx context.Context, req *mcp.CallToolRequest, args strArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.FreePort(args.Port)
		return textResult(s, err)
	})
	add("profile_up", "Start a project profile", func(ctx context.Context, req *mcp.CallToolRequest, args strArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.ProfileUp(args.Project)
		return textResult(s, err)
	})
	add("profile_down", "Stop a project profile", func(ctx context.Context, req *mcp.CallToolRequest, args strArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.ProfileDown(args.Project)
		return textResult(s, err)
	})
	add("list_skills", "List skills and rules", func(ctx context.Context, req *mcp.CallToolRequest, args strArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.ListSkills()
		return textResult(s, err)
	})
	add("get_skill", "Get a skill or rule by name", func(ctx context.Context, req *mcp.CallToolRequest, args strArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.GetSkill(args.Name)
		return textResult(s, err)
	})

	type gcpArgs struct {
		Alias        string `json:"alias,omitempty"`
		Filter       string `json:"filter,omitempty"`
		Since        string `json:"since,omitempty"`
		Severity     string `json:"severity,omitempty"`
		Limit        int    `json:"limit,omitempty"`
		SQL          string `json:"sql,omitempty"`
		Subscription string `json:"subscription,omitempty"`
		Bucket       string `json:"bucket,omitempty"`
		Prefix       string `json:"prefix,omitempty"`
		Project      string `json:"project,omitempty"`
		Grep         string `json:"grep,omitempty"`
		Shard        string `json:"shard,omitempty"`
		ID           string `json:"id,omitempty"`
	}
	addGCP := func(name, desc string, fn func(context.Context, *mcp.CallToolRequest, gcpArgs) (*mcp.CallToolResult, any, error)) {
		mcp.AddTool(server, &mcp.Tool{Name: name, Description: desc}, fn)
	}
	addGCP("gcp_list_projects", "List configured GCP aliases", func(ctx context.Context, req *mcp.CallToolRequest, args gcpArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.GCPListProjects()
		return textResult(s, err)
	})
	addGCP("gcp_logs", "Read Cloud Logging (allowlisted aliases)", func(ctx context.Context, req *mcp.CallToolRequest, args gcpArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.GCPCall(args.Alias, "logs.read", map[string]any{"filter": args.Filter, "since": args.Since, "severity": args.Severity, "limit": args.Limit})
		return textResult(s, err)
	})
	addGCP("gcp_bq_query", "Run a SELECT BigQuery (allowlisted)", func(ctx context.Context, req *mcp.CallToolRequest, args gcpArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.GCPCall(args.Alias, "bq.query", map[string]any{"sql": args.SQL})
		return textResult(s, err)
	})
	addGCP("gcp_run_status", "Cloud Run services and traffic", func(ctx context.Context, req *mcp.CallToolRequest, args gcpArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.GCPCall(args.Alias, "run.status", map[string]any{})
		return textResult(s, err)
	})
	addGCP("gcp_pubsub_peek", "Peek Pub/Sub without ack", func(ctx context.Context, req *mcp.CallToolRequest, args gcpArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.GCPCall(args.Alias, "pubsub.peek", map[string]any{"subscription": args.Subscription})
		return textResult(s, err)
	})
	addGCP("gcp_gcs_list", "List GCS objects (metadata)", func(ctx context.Context, req *mcp.CallToolRequest, args gcpArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.GCPCall(args.Alias, "gcs.list", map[string]any{"bucket": args.Bucket, "prefix": args.Prefix})
		return textResult(s, err)
	})
	addGCP("e2e_run", "Start an isolated E2E run", func(ctx context.Context, req *mcp.CallToolRequest, args gcpArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.E2ERun(args.Project, args.Grep, args.Shard)
		return textResult(s, err)
	})
	addGCP("e2e_status", "E2E run status", func(ctx context.Context, req *mcp.CallToolRequest, args gcpArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.E2EStatus(args.ID)
		return textResult(s, err)
	})
	addGCP("e2e_failures", "Failing tests and artifacts", func(ctx context.Context, req *mcp.CallToolRequest, args gcpArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.E2EFailures(args.ID)
		return textResult(s, err)
	})
	addGCP("e2e_logs", "E2E run logs", func(ctx context.Context, req *mcp.CallToolRequest, args gcpArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.E2ELogs(args.ID)
		return textResult(s, err)
	})
	addGCP("secrets_list", "Secret names and whether they resolve (never values)", func(ctx context.Context, req *mcp.CallToolRequest, args gcpArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.SecretsList()
		return textResult(s, err)
	})
	addGCP("events_recent", "Recent daemon events (crashes, E2E, GCP errors)", func(ctx context.Context, req *mcp.CallToolRequest, args gcpArgs) (*mcp.CallToolResult, any, error) {
		s, err := t.EventsRecent()
		return textResult(s, err)
	})

	return server.Run(context.Background(), &mcp.StdioTransport{})
}

func textResult(s string, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}, nil, nil
}
