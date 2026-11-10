package gcp

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/rushikesh/devhub/internal/config"
	"google.golang.org/api/bigquery/v2"
	"google.golang.org/api/logging/v2"
	"google.golang.org/api/pubsub/v1"
	"google.golang.org/api/run/v2"
	"google.golang.org/api/secretmanager/v1"
	"google.golang.org/api/storage/v1"
)

func DefaultFactory(ctx context.Context, p config.GCPProject) (*Runtime, error) {
	sa := p.ImpersonateServiceAccount
	opts, err := ClientOptions(ctx, sa)
	if err != nil {
		return nil, err
	}
	logs, err := logging.NewService(ctx, opts...)
	if err != nil {
		return nil, err
	}
	bq, err := bigquery.NewService(ctx, opts...)
	if err != nil {
		return nil, err
	}
	ps, err := pubsub.NewService(ctx, opts...)
	if err != nil {
		return nil, err
	}
	rn, err := run.NewService(ctx, opts...)
	if err != nil {
		return nil, err
	}
	sm, err := secretmanager.NewService(ctx, opts...)
	if err != nil {
		return nil, err
	}
	gcs, err := storage.NewService(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &Runtime{
		ProjectID: p.ID,
		BQMax:     ParseBytes(p.BQ.MaxBytesBilled),
		DryFirst:  p.BQ.DryRunFirst,
		Logs:      &restLogs{s: logs},
		BQ:        &restBQ{s: bq},
		Pubsub:    &restPubsub{s: ps},
		Run:       &restRun{s: rn},
		Secrets:   &restSecrets{s: sm},
		GCS:       &restGCS{s: gcs},
	}, nil
}

type restLogs struct{ s *logging.Service }

func (r *restLogs) ListEntries(ctx context.Context, projectID, filter string, pageSize int64) ([]LogEntry, error) {
	req := &logging.ListLogEntriesRequest{
		ResourceNames: []string{"projects/" + projectID},
		Filter:        filter,
		PageSize:      pageSize,
		OrderBy:       "timestamp desc",
	}
	resp, err := r.s.Entries.List(req).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	out := make([]LogEntry, 0, len(resp.Entries))
	for _, e := range resp.Entries {
		text := e.TextPayload
		if text == "" && e.JsonPayload != nil {
			b, _ := e.JsonPayload.MarshalJSON()
			text = string(b)
		}
		out = append(out, LogEntry{Timestamp: e.Timestamp, Severity: e.Severity, Text: text, InsertID: e.InsertId})
	}
	return out, nil
}

type restBQ struct{ s *bigquery.Service }

func (r *restBQ) DryRun(ctx context.Context, projectID, sql string, maxBytes int64) (int64, error) {
	legacy := false
	resp, err := r.s.Jobs.Query(projectID, &bigquery.QueryRequest{
		Query:              sql,
		UseLegacySql:       &legacy,
		DryRun:             true,
		MaximumBytesBilled: maxBytes,
	}).Context(ctx).Do()
	if err != nil {
		return 0, err
	}
	if resp.JobComplete && resp.TotalBytesProcessed != 0 {
		return resp.TotalBytesProcessed, nil
	}
	return 0, nil
}

func (r *restBQ) Query(ctx context.Context, projectID, sql string, maxBytes int64, maxRows int) ([]BQRow, error) {
	legacy := false
	resp, err := r.s.Jobs.Query(projectID, &bigquery.QueryRequest{
		Query:              sql,
		UseLegacySql:       &legacy,
		MaximumBytesBilled: maxBytes,
		MaxResults:         int64(maxRows),
	}).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return bqRows(resp, maxRows), nil
}

func bqRows(resp *bigquery.QueryResponse, maxRows int) []BQRow {
	if resp == nil || resp.Schema == nil {
		return nil
	}
	var names []string
	for _, f := range resp.Schema.Fields {
		names = append(names, f.Name)
	}
	var out []BQRow
	for i, row := range resp.Rows {
		if i >= maxRows {
			break
		}
		m := BQRow{}
		for j, c := range row.F {
			name := names[j]
			if c != nil {
				m[name] = c.V
			}
		}
		out = append(out, m)
	}
	return out
}

type restPubsub struct{ s *pubsub.Service }

func (r *restPubsub) Peek(ctx context.Context, projectID, subscription string, max int) ([]map[string]any, error) {
	name := subscription
	if !strings.Contains(name, "/") {
		name = fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subscription)
	}
	resp, err := r.s.Projects.Subscriptions.Pull(name, &pubsub.PullRequest{
		MaxMessages:       int64(max),
		ReturnImmediately: true,
	}).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	var ackIDs []string
	for _, m := range resp.ReceivedMessages {
		ackIDs = append(ackIDs, m.AckId)
		body := ""
		if m.Message != nil {
			if b, err := base64.StdEncoding.DecodeString(m.Message.Data); err == nil {
				body = string(b)
			} else {
				body = m.Message.Data
			}
		}
		out = append(out, map[string]any{
			"data":        body,
			"messageId":   m.Message.MessageId,
			"publishTime": m.Message.PublishTime,
			"attributes":  m.Message.Attributes,
		})
	}
	if len(ackIDs) > 0 {
		_, _ = r.s.Projects.Subscriptions.ModifyAckDeadline(name, &pubsub.ModifyAckDeadlineRequest{
			AckIds:             ackIDs,
			AckDeadlineSeconds: 0,
		}).Context(ctx).Do()
	}
	return out, nil
}

type restRun struct{ s *run.Service }

func (r *restRun) ListServices(ctx context.Context, projectID string) ([]RunService, error) {
	parent := fmt.Sprintf("projects/%s/locations/-", projectID)
	resp, err := r.s.Projects.Locations.Services.List(parent).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	var out []RunService
	for _, svc := range resp.Services {
		item := RunService{Name: svc.Name, URL: svc.Uri, LatestReady: svc.LatestReadyRevision}
		if svc.TrafficStatuses != nil {
			for _, t := range svc.TrafficStatuses {
				item.Traffic = append(item.Traffic, map[string]any{
					"percent":  t.Percent,
					"revision": t.Revision,
					"tag":      t.Tag,
					"type":     t.Type,
				})
			}
		}
		out = append(out, item)
	}
	return out, nil
}

type restSecrets struct{ s *secretmanager.Service }

func (r *restSecrets) Access(ctx context.Context, projectID, secret, version string) (string, error) {
	name := fmt.Sprintf("projects/%s/secrets/%s/versions/%s", projectID, secret, version)
	resp, err := r.s.Projects.Secrets.Versions.Access(name).Context(ctx).Do()
	if err != nil {
		return "", err
	}
	if resp.Payload == nil {
		return "", fmt.Errorf("empty secret payload")
	}
	return string(resp.Payload.Data), nil
}

type restGCS struct{ s *storage.Service }

func (r *restGCS) List(ctx context.Context, bucket, prefix string, max int) ([]GCSObject, error) {
	call := r.s.Objects.List(bucket).Context(ctx).Prefix(prefix).MaxResults(int64(max))
	resp, err := call.Do()
	if err != nil {
		return nil, err
	}
	var out []GCSObject
	for _, o := range resp.Items {
		out = append(out, GCSObject{
			Name: o.Name, Size: int64(o.Size), Updated: o.Updated,
			ContentType: o.ContentType, StorageClass: o.StorageClass,
		})
	}
	return out, nil
}
