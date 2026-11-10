package gcp

import (
	"context"
	"encoding/json"
	"fmt"
)

func init() { Register(gcsTask{}) }

type gcsTask struct{}

func (gcsTask) Name() string { return "gcs.list" }

type gcsParams struct {
	Bucket string `json:"bucket"`
	Prefix string `json:"prefix"`
	Max    int    `json:"max"`
}

func (gcsTask) Validate(raw json.RawMessage) error {
	p, err := decode[gcsParams](raw)
	if err != nil {
		return err
	}
	if p.Bucket == "" {
		return fmt.Errorf("bucket is required")
	}
	return nil
}

func (gcsTask) Run(ctx context.Context, rt *Runtime, raw json.RawMessage) (any, error) {
	if rt == nil || rt.GCS == nil {
		return nil, fmt.Errorf("gcs client not configured")
	}
	p, err := decode[gcsParams](raw)
	if err != nil {
		return nil, err
	}
	max := p.Max
	if max <= 0 {
		max = 200
	}
	return rt.GCS.List(ctx, p.Bucket, p.Prefix, max)
}
