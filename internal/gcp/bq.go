package gcp

import (
	"context"
	"encoding/json"
	"fmt"
)

func init() { Register(bqTask{}) }

type bqTask struct{}

func (bqTask) Name() string { return "bq.query" }

type bqParams struct {
	SQL string `json:"sql"`
}

func (bqTask) Validate(raw json.RawMessage) error {
	p, err := decode[bqParams](raw)
	if err != nil {
		return err
	}
	return ValidateSelect(p.SQL)
}

func (bqTask) Run(ctx context.Context, rt *Runtime, raw json.RawMessage) (any, error) {
	if rt == nil || rt.BQ == nil {
		return nil, fmt.Errorf("bigquery client not configured")
	}
	p, err := decode[bqParams](raw)
	if err != nil {
		return nil, err
	}
	if err := ValidateSelect(p.SQL); err != nil {
		return nil, err
	}
	maxBytes := rt.BQMax
	if maxBytes <= 0 {
		maxBytes = 1 << 30
	}
	if rt.DryFirst {
		if _, err := rt.BQ.DryRun(ctx, rt.ProjectID, p.SQL, maxBytes); err != nil {
			return nil, fmt.Errorf("dry run: %w", err)
		}
	}
	rows, err := rt.BQ.Query(ctx, rt.ProjectID, p.SQL, maxBytes, 500)
	if err != nil {
		return nil, err
	}
	if len(rows) > 500 {
		rows = rows[:500]
	}
	return rows, nil
}
