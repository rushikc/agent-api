package profile

import (
	"context"
	"time"

	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/events"
	"github.com/rushikesh/devhub/internal/registry"
)

func Supervise(ctx context.Context, svc *core.Service) {
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	backoff := map[int64]time.Time{}
	seenExit := map[int64]bool{}
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
					if sess.Status == registry.StatusRunning {
						seenExit[sess.ID] = false
						if sh.Restart != "always" {
							continue
						}
						continue
					}
					if !seenExit[sess.ID] {
						seenExit[sess.ID] = true
						svc.Emit(events.ShadowCrashed, loaded.Name+"/"+sh.Name, loaded.Name, map[string]any{"name": sh.Name})
					}
					if sess.RestartCount >= 5 {
						svc.Emit(events.ShadowRestartedMax, loaded.Name+"/"+sh.Name, loaded.Name, map[string]any{"name": sh.Name})
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
