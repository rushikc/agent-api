package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rushikesh/devhub/internal/client"
	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/daemon"
)

type StartShadowArgs struct {
	Project string   `json:"project"`
	Command string   `json:"command,omitempty"`
	Argv    []string `json:"argv,omitempty"`
	Name    string   `json:"name,omitempty"`
}

type EmptyArgs struct{}

type AliasArgs struct {
	Alias string `json:"alias"`
}

type GCPLogsArgs struct {
	Alias    string `json:"alias"`
	Filter   string `json:"filter,omitempty"`
	Since    string `json:"since,omitempty"`
	Severity string `json:"severity,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type GCPBQArgs struct {
	Alias string `json:"alias"`
	SQL   string `json:"sql"`
}

type GCPPubsubArgs struct {
	Alias        string `json:"alias"`
	Subscription string `json:"subscription,omitempty"`
}

type GCPGCSArgs struct {
	Alias  string `json:"alias"`
	Bucket string `json:"bucket,omitempty"`
	Prefix string `json:"prefix,omitempty"`
}

type E2ERunArgs struct {
	Project string `json:"project"`
	Grep    string `json:"grep,omitempty"`
	Shard   string `json:"shard,omitempty"`
}

type E2EIDArgs struct {
	ID string `json:"id"`
}

func RunStdio(cfg *config.Config) error {
	_ = daemon.Ensure(cfg)
	c := client.Unix(cfg, "mcp")
	t := &Tools{C: c, Cfg: cfg}
	server := mcp.NewServer(&mcp.Implementation{Name: "devhub", Version: "3.0.0"}, nil)

	type listSessionsArgs struct {
		Project string `json:"project,omitempty"`
		Kind    string `json:"kind,omitempty"`
	}
	type startShadowArgs = StartShadowArgs
	type idArgs struct {
		ID int64 `json:"id"`
	}
	type logsArgs struct {
		ID    int64  `json:"id"`
		Lines int    `json:"lines,omitempty"`
		Grep  string `json:"grep,omitempty"`
	}
	type portArgs struct {
		Port int `json:"port"`
	}
	type projectArgs struct {
		Project string `json:"project"`
	}
	type nameArgs struct {
		Name string `json:"name"`
	}

	mcp.AddTool(server, &mcp.Tool{Name: "list_sessions", Description: "List devhub sessions"}, func(ctx context.Context, req *mcp.CallToolRequest, args listSessionsArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.ListSessions(args.Project, args.Kind) })
	})
	mcp.AddTool(server, &mcp.Tool{Name: "start_shadow", Description: "Start a shadow process. Prefer argv (array). command is a string run via /bin/sh -c."}, func(ctx context.Context, req *mcp.CallToolRequest, args startShadowArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.StartShadow(args.Project, args.Command, args.Argv, args.Name) })
	})
	mcp.AddTool(server, &mcp.Tool{Name: "restart_session", Description: "Restart a session by id"}, func(ctx context.Context, req *mcp.CallToolRequest, args idArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.RestartSession(args.ID) })
	})
	mcp.AddTool(server, &mcp.Tool{Name: "stop_session", Description: "Stop a session by id"}, func(ctx context.Context, req *mcp.CallToolRequest, args idArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.StopSession(args.ID) })
	})
	mcp.AddTool(server, &mcp.Tool{Name: "read_logs", Description: "Read session logs (secrets scrubbed)"}, func(ctx context.Context, req *mcp.CallToolRequest, args logsArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.ReadLogs(args.ID, args.Lines, args.Grep) })
	})
	mcp.AddTool(server, &mcp.Tool{Name: "get_stats", Description: "Resource snapshot"}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.GetStats() })
	})
	mcp.AddTool(server, &mcp.Tool{Name: "list_ports", Description: "Listening TCP ports"}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.ListPorts() })
	})
	mcp.AddTool(server, &mcp.Tool{Name: "free_port", Description: "Free a TCP port (requires mcp.allow_port_kill)"}, func(ctx context.Context, req *mcp.CallToolRequest, args portArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.FreePort(args.Port) })
	})
	mcp.AddTool(server, &mcp.Tool{Name: "profile_up", Description: "Start a project profile"}, func(ctx context.Context, req *mcp.CallToolRequest, args projectArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.ProfileUp(args.Project) })
	})
	mcp.AddTool(server, &mcp.Tool{Name: "profile_down", Description: "Stop a project profile"}, func(ctx context.Context, req *mcp.CallToolRequest, args projectArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.ProfileDown(args.Project) })
	})
	mcp.AddTool(server, &mcp.Tool{Name: "list_skills", Description: "List skills and rules"}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.ListSkills() })
	})
	mcp.AddTool(server, &mcp.Tool{Name: "get_skill", Description: "Get a skill or rule by name"}, func(ctx context.Context, req *mcp.CallToolRequest, args nameArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.GetSkill(args.Name) })
	})

	mcp.AddTool(server, &mcp.Tool{Name: "gcp_list_projects", Description: "List configured GCP aliases"}, func(ctx context.Context, req *mcp.CallToolRequest, args EmptyArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.GCPListProjects() })
	})
	mcp.AddTool(server, &mcp.Tool{Name: "gcp_logs", Description: "Read Cloud Logging (allowlisted aliases)"}, func(ctx context.Context, req *mcp.CallToolRequest, args GCPLogsArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) {
			return t.GCPCall(args.Alias, "logs.read", map[string]any{"filter": args.Filter, "since": args.Since, "severity": args.Severity, "limit": args.Limit})
		})
	})
	mcp.AddTool(server, &mcp.Tool{Name: "gcp_bq_query", Description: "Run a SELECT BigQuery (allowlisted)"}, func(ctx context.Context, req *mcp.CallToolRequest, args GCPBQArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.GCPCall(args.Alias, "bq.query", map[string]any{"sql": args.SQL}) })
	})
	mcp.AddTool(server, &mcp.Tool{Name: "gcp_run_status", Description: "Cloud Run services and traffic"}, func(ctx context.Context, req *mcp.CallToolRequest, args AliasArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.GCPCall(args.Alias, "run.status", map[string]any{}) })
	})
	mcp.AddTool(server, &mcp.Tool{Name: "gcp_pubsub_peek", Description: "Peek Pub/Sub without ack"}, func(ctx context.Context, req *mcp.CallToolRequest, args GCPPubsubArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) {
			return t.GCPCall(args.Alias, "pubsub.peek", map[string]any{"subscription": args.Subscription})
		})
	})
	mcp.AddTool(server, &mcp.Tool{Name: "gcp_gcs_list", Description: "List GCS objects (metadata)"}, func(ctx context.Context, req *mcp.CallToolRequest, args GCPGCSArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) {
			return t.GCPCall(args.Alias, "gcs.list", map[string]any{"bucket": args.Bucket, "prefix": args.Prefix})
		})
	})
	mcp.AddTool(server, &mcp.Tool{Name: "e2e_run", Description: "Start an isolated E2E run"}, func(ctx context.Context, req *mcp.CallToolRequest, args E2ERunArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.E2ERun(args.Project, args.Grep, args.Shard) })
	})
	mcp.AddTool(server, &mcp.Tool{Name: "e2e_status", Description: "E2E run status"}, func(ctx context.Context, req *mcp.CallToolRequest, args E2EIDArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.E2EStatus(args.ID) })
	})
	mcp.AddTool(server, &mcp.Tool{Name: "e2e_failures", Description: "Failing tests and artifacts"}, func(ctx context.Context, req *mcp.CallToolRequest, args E2EIDArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.E2EFailures(args.ID) })
	})
	mcp.AddTool(server, &mcp.Tool{Name: "e2e_logs", Description: "E2E run logs"}, func(ctx context.Context, req *mcp.CallToolRequest, args E2EIDArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.E2ELogs(args.ID) })
	})
	mcp.AddTool(server, &mcp.Tool{Name: "secrets_list", Description: "Secret names and whether they resolve (never values)"}, func(ctx context.Context, req *mcp.CallToolRequest, args EmptyArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.SecretsList() })
	})
	mcp.AddTool(server, &mcp.Tool{Name: "events_recent", Description: "Recent daemon events (crashes, E2E, GCP errors)"}, func(ctx context.Context, req *mcp.CallToolRequest, args EmptyArgs) (*mcp.CallToolResult, any, error) {
		return safeText(func() (string, error) { return t.EventsRecent() })
	})

	return server.Run(context.Background(), &mcp.StdioTransport{})
}

func textResult(s string, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}, nil, nil
}

func safeText(fn func() (string, error)) (res *mcp.CallToolResult, _ any, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			res, _, err = textResult("", fmt.Errorf("panic: %v", rec))
		}
	}()
	return textResult(fn())
}
