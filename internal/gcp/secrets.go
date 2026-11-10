package gcp

import (
	"context"
	"encoding/json"
	"fmt"
)

func init() { Register(secretsTask{}) }

type secretsTask struct{}

func (secretsTask) Name() string { return "secrets.read" }

type secretsParams struct {
	Secret  string `json:"secret"`
	Version string `json:"version"`
}

func (secretsTask) Validate(raw json.RawMessage) error {
	p, err := decode[secretsParams](raw)
	if err != nil {
		return err
	}
	if p.Secret == "" {
		return fmt.Errorf("secret is required")
	}
	return nil
}

func (secretsTask) Run(ctx context.Context, rt *Runtime, raw json.RawMessage) (any, error) {
	if rt == nil || rt.Secrets == nil {
		return nil, fmt.Errorf("secret manager client not configured")
	}
	p, err := decode[secretsParams](raw)
	if err != nil {
		return nil, err
	}
	ver := p.Version
	if ver == "" {
		ver = "latest"
	}
	val, err := rt.Secrets.Access(ctx, rt.ProjectID, p.Secret, ver)
	if err != nil {
		return nil, err
	}
	return secretValue{Value: val}, nil
}

type secretValue struct {
	Value string `json:"-"`
}
