package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

type Task interface {
	Name() string
	Validate(json.RawMessage) error
	Run(ctx context.Context, rt *Runtime, params json.RawMessage) (any, error)
}

var (
	regMu sync.RWMutex
	tasks = map[string]Task{}
)

func Register(t Task) {
	regMu.Lock()
	defer regMu.Unlock()
	tasks[t.Name()] = t
}

func Lookup(name string) (Task, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	t, ok := tasks[name]
	return t, ok
}

func Names() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(tasks))
	for n := range tasks {
		out = append(out, n)
	}
	return out
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	if len(raw) == 0 {
		return v, nil
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, fmt.Errorf("params: %w", err)
	}
	return v, nil
}
