package profile

import (
	"testing"

	"github.com/rushikesh/devhub/internal/events"
	"github.com/rushikesh/devhub/internal/registry"
)

func TestTickManualShadowCrash(t *testing.T) {
	svc := testProfileService(t)
	var kinds []string
	svc.Publish = func(kind, target, project string, payload map[string]any) {
		kinds = append(kinds, kind)
	}
	sess, err := svc.NewShadow("krayon", "job", []string{"false"})
	if err != nil {
		t.Fatal(err)
	}
	code := 1
	sess.Status = registry.StatusExited
	sess.ExitCode = &code
	if err := svc.DB.Update(sess); err != nil {
		t.Fatal(err)
	}
	seen := map[int64]string{}
	Tick(svc, nil, seen)
	if len(kinds) != 1 || kinds[0] != events.ShadowCrashed {
		t.Fatalf("%v", kinds)
	}
	Tick(svc, nil, seen)
	if len(kinds) != 1 {
		t.Fatalf("duplicate %v", kinds)
	}
}

func TestTickKillDoesNotAlert(t *testing.T) {
	svc := testProfileService(t)
	var kinds []string
	svc.Publish = func(kind, target, project string, payload map[string]any) {
		kinds = append(kinds, kind)
	}
	if _, err := svc.NewShadow("krayon", "job", []string{"true"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Kill("krayon/job"); err != nil {
		t.Fatal(err)
	}
	Tick(svc, nil, map[int64]string{})
	for _, k := range kinds {
		if k == events.ShadowCrashed {
			t.Fatal(kinds)
		}
	}
}

func TestTickCleanExit(t *testing.T) {
	svc := testProfileService(t)
	var kinds []string
	svc.Publish = func(kind, target, project string, payload map[string]any) {
		kinds = append(kinds, kind)
	}
	sess, err := svc.NewShadow("krayon", "job", []string{"true"})
	if err != nil {
		t.Fatal(err)
	}
	z := 0
	sess.Status = registry.StatusExited
	sess.ExitCode = &z
	_ = svc.DB.Update(sess)
	Tick(svc, nil, map[int64]string{})
	if len(kinds) != 1 || kinds[0] != events.ShadowExited {
		t.Fatalf("%v", kinds)
	}
}

func TestTickDoesNotRestartManualWithoutPolicy(t *testing.T) {
	svc := testProfileService(t)
	sess, err := svc.NewShadow("krayon", "job", []string{"false"})
	if err != nil {
		t.Fatal(err)
	}
	code := 1
	sess.Status = registry.StatusExited
	sess.ExitCode = &code
	_ = svc.DB.Update(sess)
	Tick(svc, nil, map[int64]string{})
	got, _ := svc.DB.Get(sess.ID)
	if got.RestartCount != 0 {
		t.Fatalf("restarted %+v", got)
	}
}
