package e2e

import (
	"context"
	"io"
)

type Engine interface {
	RuntimeName() string
	HasImage(ctx context.Context, tag string) bool
	BuildImage(ctx context.Context, dockerfileDir, dockerfile, tag string, w io.Writer) error
	CreateNetwork(ctx context.Context, name string) (string, error)
	RemoveNetwork(ctx context.Context, id string) error
	EnsureVolume(ctx context.Context, name string) error
	CreateContainer(ctx context.Context, spec ContainerSpec) (string, error)
	StartContainer(ctx context.Context, id string) error
	WaitContainer(ctx context.Context, id string) (int, error)
	Logs(ctx context.Context, id string) (io.ReadCloser, error)
	CopyTo(ctx context.Context, id, dest string, tar io.Reader) error
	CopyFrom(ctx context.Context, id, src string) (io.ReadCloser, error)
	RemoveContainer(ctx context.Context, id string) error
	ContainerIP(ctx context.Context, id string) (string, error)
	PruneImages(ctx context.Context, prefix string) error
	PruneVolumes(ctx context.Context, prefix string) error
}

type ContainerSpec struct {
	Name       string
	Image      string
	Cmd        []string
	Env        map[string]string
	Network    string
	Hostname   string
	Binds      []string
	CPUs       float64
	Memory     int64
	WorkDir    string
	AutoRemove bool
}
