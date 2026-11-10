package gcp

import (
	"context"
	"encoding/json"
	"fmt"
)

func init() { Register(pubsubTask{}) }

type pubsubTask struct{}

func (pubsubTask) Name() string { return "pubsub.peek" }

type pubsubParams struct {
	Subscription string `json:"subscription"`
	Max          int    `json:"max"`
}

func (pubsubTask) Validate(raw json.RawMessage) error {
	p, err := decode[pubsubParams](raw)
	if err != nil {
		return err
	}
	if p.Subscription == "" {
		return fmt.Errorf("subscription is required")
	}
	if p.Max > 20 {
		return fmt.Errorf("max is 20")
	}
	return nil
}

func (pubsubTask) Run(ctx context.Context, rt *Runtime, raw json.RawMessage) (any, error) {
	if rt == nil || rt.Pubsub == nil {
		return nil, fmt.Errorf("pubsub client not configured")
	}
	p, err := decode[pubsubParams](raw)
	if err != nil {
		return nil, err
	}
	max := p.Max
	if max <= 0 {
		max = 20
	}
	if max > 20 {
		max = 20
	}
	return rt.Pubsub.Peek(ctx, rt.ProjectID, p.Subscription, max)
}
