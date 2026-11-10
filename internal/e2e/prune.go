package e2e

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/rushikesh/devhub/internal/registry"
)

func (q *Queue) Prune(ctx context.Context) error {
	if q.Eng != nil {
		_ = q.Eng.PruneImages(ctx, "devhub/")
		_ = q.Eng.PruneVolumes(ctx, "dh-e2e-")
	}
	if q.Cfg == nil {
		return nil
	}
	root := q.Cfg.E2EDir()
	retain := 7 * 24 * time.Hour
	if q.Cfg.E2E.Retention != "" {
		if d, err := time.ParseDuration(q.Cfg.E2E.Retention); err == nil {
			retain = d
		}
	}
	cutoff := time.Now().Add(-retain)
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || path == root {
			return nil
		}
		if info.IsDir() && info.ModTime().Before(cutoff) && path != root {
			_ = os.RemoveAll(path)
			return filepath.SkipDir
		}
		return nil
	})
	return nil
}

func (q *Queue) Build(ctx context.Context, prDir, dockerfile, image string) (string, error) {
	if err := q.requireEngine(); err != nil {
		q.failEvent(registry.E2ERun{Project: filepath.Base(prDir)}, err)
		return "", err
	}
	df := dockerfile
	if df == "" {
		df = "e2e/Dockerfile"
	}
	if !filepath.IsAbs(df) {
		df = filepath.Join(prDir, df)
	}
	tag, err := ImageTag(image, df, lockfiles(prDir))
	if err != nil {
		return "", err
	}
	if err := q.Eng.BuildImage(ctx, filepath.Dir(df), filepath.Base(df), tag, os.Stdout); err != nil {
		return tag, err
	}
	return tag, nil
}
