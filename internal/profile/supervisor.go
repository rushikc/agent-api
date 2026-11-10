package profile

import (
	"context"
	"strings"
	"time"

	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/events"
	"github.com/rushikesh/devhub/internal/registry"
	"github.com/rushikesh/devhub/internal/scrub"
)

func ShouldRestart(sess registry.Session, policy string) bool {
	if sess.Status == registry.StatusStopped || sess.Status == registry.StatusRunning {
		return false
	}
	return policy == "on-failure" || policy == "always"
}

func profilePolicies(cfgList []Profile) map[string]string {
	out := map[string]string{}
	for _, pr := range cfgList {
		if pr.Path == "" {
			continue
		}
		loaded, err := Load(pr.Path)
		if err != nil {
			continue
		}
		name := loaded.Name
		if name == "" {
			name = pr.Name
		}
		for _, sh := range loaded.Shadows {
			out[name+"/"+sh.Name] = sh.Restart
		}
	}
	return out
}

func lastLogLines(svc *core.Service, sess registry.Session, n int) string {
	out, err := svc.Peek(sess.Project + "/" + sess.Name)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return scrub.Scrub(strings.Join(lines, "\n"))
}

func exitCode(sess registry.Session) int {
	if sess.ExitCode == nil {
		return 0
	}
	return *sess.ExitCode
}

func Tick(svc *core.Service, policies map[string]string, seen map[int64]string) {
	if svc == nil || svc.DB == nil {
		return
	}
	rows, err := svc.DB.List(true)
	if err != nil {
		return
	}
	for _, sess := range rows {
		if sess.Kind != registry.KindShadow {
			continue
		}
		key := sess.Project + "/" + sess.Name
		policy := sess.Restart
		if policy == "" {
			policy = policies[key]
		}
		switch sess.Status {
		case registry.StatusRunning:
			seen[sess.ID] = ""
		case registry.StatusStopped:
			seen[sess.ID] = "stopped"
		case registry.StatusExited:
			code := exitCode(sess)
			kind := events.ShadowExited
			if code != 0 {
				kind = events.ShadowCrashed
			}
			if seen[sess.ID] != kind {
				payload := map[string]any{"name": sess.Name, "exit_code": code, "logs": lastLogLines(svc, sess, 20)}
				svc.Emit(kind, key, sess.Project, payload)
				seen[sess.ID] = kind
			}
			if ShouldRestart(sess, policy) {
				if sess.RestartCount >= 5 {
					svc.Emit(events.ShadowRestartedMax, key, sess.Project, map[string]any{"name": sess.Name})
					continue
				}
				_ = svc.Restart(key)
			}
		}
	}
}

func Supervise(ctx context.Context, svc *core.Service) {
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	seen := map[int64]string{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			Tick(svc, profilePolicies(List(svc.Cfg)), seen)
		}
	}
}
