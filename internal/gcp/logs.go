package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func init() { Register(logsTask{}) }

type logsTask struct{}

func (logsTask) Name() string { return "logs.read" }

type logsParams struct {
	Filter   string `json:"filter"`
	Since    string `json:"since"`
	Severity string `json:"severity"`
	Limit    int    `json:"limit"`
}

func (logsTask) Validate(raw json.RawMessage) error {
	p, err := decode[logsParams](raw)
	if err != nil {
		return err
	}
	if p.Limit > 1000 {
		return fmt.Errorf("limit max is 1000")
	}
	return nil
}

func (t logsTask) Run(ctx context.Context, rt *Runtime, raw json.RawMessage) (any, error) {
	if rt == nil || rt.Logs == nil {
		return nil, fmt.Errorf("logging client not configured")
	}
	p, err := decode[logsParams](raw)
	if err != nil {
		return nil, err
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	filter := strings.TrimSpace(p.Filter)
	if p.Severity != "" {
		sev := fmt.Sprintf(`severity>=%s`, strings.ToUpper(p.Severity))
		if filter == "" {
			filter = sev
		} else {
			filter = filter + " AND " + sev
		}
	}
	if p.Since != "" {
		d, err := time.ParseDuration(p.Since)
		if err != nil {
			return nil, fmt.Errorf("since: %w", err)
		}
		ts := time.Now().Add(-d).UTC().Format(time.RFC3339)
		part := fmt.Sprintf(`timestamp>="%s"`, ts)
		if filter == "" {
			filter = part
		} else {
			filter = filter + " AND " + part
		}
	}
	return rt.Logs.ListEntries(ctx, rt.ProjectID, filter, int64(limit))
}
