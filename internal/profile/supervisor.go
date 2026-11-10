package profile

import (
	"context"
	"time"

	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/registry"
)

func Supervise(ctx context.Context, svc *core.Service) {
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	backoff := map[int64]time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			for _, pr := range List(svc.Cfg) {
				if pr.Path == "" {
					continue
				}
				loaded, err := Load(pr.Path)
				if err != nil {
					continue
				}
				for _, sh := range loaded.Shadows {
					if sh.Restart != "on-failure" && sh.Restart != "always" {
						continue
					}
					sess, err := svc.DB.GetByProjectName(loaded.Name, sh.Name)
					if err != nil {
						continue
					}
					if sess.Status == registry.StatusRunning && sh.Restart != "always" {
						continue
					}
					if sess.Status == registry.StatusRunning {
						continue
					}
					if sess.RestartCount >= 5 {
						continue
					}
					if until, ok := backoff[sess.ID]; ok && time.Now().Before(until) {
						continue
					}
					_ = svc.Restart(loaded.Name + "/" + sh.Name)
					d := time.Duration(1<<min(sess.RestartCount, 4)) * time.Second
					backoff[sess.ID] = time.Now().Add(d)
				}
			}
		}
	}
}
