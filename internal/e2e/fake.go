package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
)

type FakeEngine struct {
	Mu           sync.Mutex
	Images       map[string]bool
	Networks     map[string]bool
	Volumes      map[string]bool
	Containers   map[string]ContainerSpec
	Started      []string
	Removed      []string
	NetsRemoved  []string
	FailCreate   bool
	FailStart    bool
	ExitCode     int
	LogBody      string
	CopyFromErr  bool
	CopyFromData map[string][]byte
	WaitFn       func(ctx context.Context, id string) (int, error)
}

func NewFake() *FakeEngine {
	return &FakeEngine{
		Images:     map[string]bool{},
		Networks:   map[string]bool{},
		Volumes:    map[string]bool{},
		Containers: map[string]ContainerSpec{},
		LogBody:    "ok\n",
	}
}

func (f *FakeEngine) Ping() error         { return nil }
func (f *FakeEngine) RuntimeName() string { return "fake" }

func (f *FakeEngine) HasImage(_ context.Context, tag string) bool {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	return f.Images[tag]
}

func (f *FakeEngine) BuildImage(_ context.Context, _, _, tag string, w io.Writer) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.Images[tag] = true
	if w != nil {
		_, _ = io.WriteString(w, "built "+tag+"\n")
	}
	return nil
}

func (f *FakeEngine) CreateNetwork(_ context.Context, name string) (string, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.Networks[name] = true
	return name, nil
}

func (f *FakeEngine) RemoveNetwork(_ context.Context, id string) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	delete(f.Networks, id)
	f.NetsRemoved = append(f.NetsRemoved, id)
	return nil
}

func (f *FakeEngine) EnsureVolume(_ context.Context, name string) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.Volumes[name] = true
	return nil
}

func (f *FakeEngine) CreateContainer(_ context.Context, spec ContainerSpec) (string, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.FailCreate {
		return "", fmt.Errorf("create failed")
	}
	id := "c-" + spec.Name
	f.Containers[id] = spec
	return id, nil
}

func (f *FakeEngine) StartContainer(_ context.Context, id string) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.FailStart {
		return fmt.Errorf("start failed")
	}
	f.Started = append(f.Started, id)
	return nil
}

func (f *FakeEngine) WaitContainer(ctx context.Context, id string) (int, error) {
	if f.WaitFn != nil {
		return f.WaitFn(ctx, id)
	}
	select {
	case <-ctx.Done():
		return -1, ctx.Err()
	default:
	}
	return f.ExitCode, nil
}

func (f *FakeEngine) Logs(_ context.Context, id string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.LogBody)), nil
}

func (f *FakeEngine) CopyTo(_ context.Context, id, dest string, tar io.Reader) error {
	_, _ = io.Copy(io.Discard, tar)
	return nil
}

func (f *FakeEngine) CopyFrom(_ context.Context, id, src string) (io.ReadCloser, error) {
	if f.CopyFromErr {
		return nil, fmt.Errorf("no artifact")
	}
	if f.CopyFromData != nil {
		if b, ok := f.CopyFromData[src]; ok {
			return io.NopCloser(bytes.NewReader(b)), nil
		}
	}
	return io.NopCloser(bytes.NewReader(nil)), nil
}

func (f *FakeEngine) RemoveContainer(_ context.Context, id string) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	delete(f.Containers, id)
	f.Removed = append(f.Removed, id)
	return nil
}

func (f *FakeEngine) ContainerIP(_ context.Context, id string) (string, error) {
	return "127.0.0.1", nil
}

func (f *FakeEngine) PruneImages(_ context.Context, prefix string) error  { return nil }
func (f *FakeEngine) PruneVolumes(_ context.Context, prefix string) error { return nil }
