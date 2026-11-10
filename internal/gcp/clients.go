package gcp

import (
	"context"
	"time"
)

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Severity  string `json:"severity"`
	Text      string `json:"text"`
	InsertID  string `json:"insertId,omitempty"`
}

type LoggingAPI interface {
	ListEntries(ctx context.Context, projectID, filter string, pageSize int64) ([]LogEntry, error)
}

type BQRow map[string]any

type BigQueryAPI interface {
	DryRun(ctx context.Context, projectID, sql string, maxBytes int64) (bytesProcessed int64, err error)
	Query(ctx context.Context, projectID, sql string, maxBytes int64, maxRows int) ([]BQRow, error)
}

type PubsubAPI interface {
	Peek(ctx context.Context, projectID, subscription string, max int) ([]map[string]any, error)
}

type RunService struct {
	Name        string           `json:"name"`
	URL         string           `json:"url"`
	LatestReady string           `json:"latestReadyRevision"`
	Traffic     []map[string]any `json:"traffic"`
	Revisions   []map[string]any `json:"revisions,omitempty"`
}

type RunAPI interface {
	ListServices(ctx context.Context, projectID string) ([]RunService, error)
}

type SecretAPI interface {
	Access(ctx context.Context, projectID, secret, version string) (string, error)
}

type GCSObject struct {
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	Updated      string `json:"updated"`
	ContentType  string `json:"contentType,omitempty"`
	StorageClass string `json:"storageClass,omitempty"`
}

type GCSAPI interface {
	List(ctx context.Context, bucket, prefix string, max int) ([]GCSObject, error)
}

type AuthInfo struct {
	Email    string   `json:"email"`
	Projects []string `json:"reachable,omitempty"`
}

type AuthAPI interface {
	Check(ctx context.Context) (AuthInfo, error)
	Reachable(ctx context.Context, projectID string) bool
}

type Runtime struct {
	ProjectID string
	BQMax     int64
	DryFirst  bool
	Logs      LoggingAPI
	BQ        BigQueryAPI
	Pubsub    PubsubAPI
	Run       RunAPI
	Secrets   SecretAPI
	GCS       GCSAPI
}

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }
