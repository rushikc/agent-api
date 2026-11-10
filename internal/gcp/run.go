package gcp

import (
	"context"
	"encoding/json"
	"fmt"
)

func init() { Register(runTask{}) }

type runTask struct{}

func (runTask) Name() string { return "run.status" }

func (runTask) Validate(json.RawMessage) error { return nil }

func (runTask) Run(ctx context.Context, rt *Runtime, _ json.RawMessage) (any, error) {
	if rt == nil || rt.Run == nil {
		return nil, fmt.Errorf("cloud run client not configured")
	}
	return rt.Run.ListServices(ctx, rt.ProjectID)
}
