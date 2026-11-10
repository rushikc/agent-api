package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rushikesh/devhub/internal/tmux"
)

func tempDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "devhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestInsertAndList(t *testing.T) {
	db := tempDB(t)
	id, err := db.Insert(Session{
		Project:    "krayon",
		Name:       "v1",
		Kind:       KindActive,
		Command:    "",
		Cwd:        "/tmp",
		TmuxTarget: "dh-krayon:v1",
		PanePID:    42,
		Status:     StatusRunning,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "v1" || got.Project != "krayon" || got.PanePID != 42 {
		t.Fatalf("%+v", got)
	}
	all, err := db.List(false)
	if err != nil || len(all) != 1 {
		t.Fatalf("%v %#v", err, all)
	}
}

func TestUniqueProjectName(t *testing.T) {
	db := tempDB(t)
	s := Session{Project: "krayon", Name: "v1", Kind: KindActive, Status: StatusRunning}
	if _, err := db.Insert(s); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Insert(s); err == nil {
		t.Fatal("expected unique constraint")
	}
	s.Name = "v2"
	if _, err := db.Insert(s); err != nil {
		t.Fatal(err)
	}
}

func TestReconcile(t *testing.T) {
	db := tempDB(t)
	running := Session{Project: "krayon", Name: "v1", Kind: KindActive, TmuxTarget: "dh-krayon:v1", PanePID: 11, Status: StatusRunning}
	dead := Session{Project: "krayon", Name: "dev", Kind: KindShadow, TmuxTarget: "dh-shadow-krayon:dev", PanePID: 22, Status: StatusRunning}
	missing := Session{Project: "krayon", Name: "v2", Kind: KindActive, TmuxTarget: "dh-krayon:v2", PanePID: 33, Status: StatusRunning}
	for _, s := range []Session{running, dead, missing} {
		if _, err := db.Insert(s); err != nil {
			t.Fatal(err)
		}
	}

	panes := []tmux.Pane{
		{Session: "dh-krayon", Window: "v1", PID: 111, Target: "dh-krayon:v1"},
		{Session: "dh-shadow-krayon", Window: "dev", PID: 0, Dead: true, DeadStatus: 7, Target: "dh-shadow-krayon:dev"},
	}
	if err := db.Reconcile(panes); err != nil {
		t.Fatal(err)
	}

	v1, err := db.GetByProjectName("krayon", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if v1.Status != StatusRunning || v1.PanePID != 111 {
		t.Fatalf("running pane: %+v", v1)
	}

	dev, err := db.GetByProjectName("krayon", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if dev.Status != StatusExited || dev.ExitCode == nil || *dev.ExitCode != 7 {
		t.Fatalf("dead pane: %+v", dev)
	}

	v2, err := db.GetByProjectName("krayon", "v2")
	if err != nil {
		t.Fatal(err)
	}
	if v2.Status != StatusExited {
		t.Fatalf("missing pane should be exited: %+v", v2)
	}
}

func TestReconcileKeepsStopped(t *testing.T) {
	db := tempDB(t)
	if _, err := db.Insert(Session{Project: "krayon", Name: "web", Kind: KindShadow, Status: StatusStopped, TmuxTarget: "dh-shadow-krayon:@9"}); err != nil {
		t.Fatal(err)
	}
	if err := db.Reconcile(nil); err != nil {
		t.Fatal(err)
	}
	s, _ := db.GetByProjectName("krayon", "web")
	if s.Status != StatusStopped {
		t.Fatalf("stopped flipped to %s", s.Status)
	}
}

func TestReconcileFakeTmux(t *testing.T) {
	db := tempDB(t)
	fake := tmux.NewFake()
	if err := fake.NewSession(tmux.SessionOpts{Name: "dh-krayon", Window: "v1", Cwd: "/tmp"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Insert(Session{Project: "krayon", Name: "v1", Kind: KindActive, TmuxTarget: "dh-krayon:v1", Status: StatusRunning}); err != nil {
		t.Fatal(err)
	}
	panes, err := fake.ListPanes()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Reconcile(panes); err != nil {
		t.Fatal(err)
	}
	s, _ := db.GetByProjectName("krayon", "v1")
	if s.Status != StatusRunning || s.PanePID == 0 {
		t.Fatalf("%+v", s)
	}
	fake.MarkDead("dh-krayon:v1", 1)
	panes, _ = fake.ListPanes()
	if err := db.Reconcile(panes); err != nil {
		t.Fatal(err)
	}
	s, _ = db.GetByProjectName("krayon", "v1")
	if s.Status != StatusExited || s.ExitCode == nil || *s.ExitCode != 1 {
		t.Fatalf("%+v", s)
	}
}

func TestSecretNamesNeverStoreValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "devhub.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	secret := "supersecret-value-xyz"
	if err := db.PutSecretName("api"); err != nil {
		t.Fatal(err)
	}
	names, err := db.ListSecretNames()
	if err != nil || len(names) != 1 || names[0] != "api" {
		t.Fatalf("%v %v", names, err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), secret) {
		t.Fatal("value in db")
	}
	if err := db.DeleteSecretName("api"); err != nil {
		t.Fatal(err)
	}
	names, _ = db.ListSecretNames()
	if len(names) != 0 {
		t.Fatalf("%v", names)
	}
}
