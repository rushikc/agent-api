package profile

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/janitor"
	"github.com/rushikesh/devhub/internal/monitor"
	"github.com/rushikesh/devhub/internal/registry"
	"github.com/rushikesh/devhub/internal/tmux"
)

func TestDetectPortTable(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		file, body string
		want       int
	}{
		{"package.json", `{"scripts":{"dev":"next dev --port 4173"}}`, 4173},
		{".env", "PORT=8080\n", 8080},
		{"vite.config.ts", "export default { server: { port: 5173 } }", 5173},
		{"server.js", "app.listen(4000)\n", 4000},
		{"index.js", "http.createServer().listen(5000)\n", 5000},
		{"start.sh", "node --port=6000\n", 6000},
		{"run.sh", "node --port 7000\n", 7000},
		{"flag.sh", "node -p 8000\n", 8000},
		{"app.properties", "server.port=9000\n", 9000},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			sub := t.TempDir()
			if err := os.WriteFile(filepath.Join(sub, tc.file), []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := DetectPort(sub); got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
	_ = dir
}

func TestDetectPortPrefersEnvOverPackageJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"dev":"next --port 4173"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PORT=1234\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := DetectPort(dir); got != 1234 {
		t.Fatalf("got %d", got)
	}
}

func TestInitUsesRegisteredProjectName(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Projects = map[string]string{"demo": dir}
	pr, err := Init(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if pr.Name != "demo" {
		t.Fatalf("name %s", pr.Name)
	}
}

func TestShouldFreePortForeign(t *testing.T) {
	foreign := janitor.Port{Port: 3000, PID: 99, Owner: "external"}
	if ShouldFreePort(foreign, "krayon", nil) {
		t.Fatal("must not free foreign")
	}
	own := janitor.Port{Port: 3000, PID: 1, Owner: "krayon/web", Project: "krayon"}
	if !ShouldFreePort(own, "krayon", nil) {
		t.Fatal("should free own")
	}
}

func TestUpForeignPortDoesNotKill(t *testing.T) {
	killed := false
	oldKill, oldTaken := killTreeFn, portTakenFn
	t.Cleanup(func() { killTreeFn, portTakenFn = oldKill, oldTaken })
	killTreeFn = func(pid int32) error {
		killed = true
		return nil
	}
	portTakenFn = func(port int, sessions []registry.Session, src monitor.ProcSource) (janitor.Port, bool) {
		return janitor.Port{Port: port, PID: 4242, Owner: "external"}, true
	}
	svc := testProfileService(t)
	pr := &Profile{Name: "krayon", Path: svc.Cfg.Projects["krayon"], Shadows: []Shadow{{Name: "web", Command: "true", Port: 3000}}}
	if _, err := Up(svc, pr, true); err == nil {
		t.Fatal("expected foreign port error")
	}
	if killed {
		t.Fatal("KillTree must not run for foreign listener")
	}
}

func TestDownStopsShadowsOnly(t *testing.T) {
	svc := testProfileService(t)
	if _, err := svc.NewActive("krayon", "v1"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.NewShadow("krayon", "web", []string{"sleep", "9"}); err != nil {
		t.Fatal(err)
	}
	if err := Down(svc, "krayon", false); err != nil {
		t.Fatal(err)
	}
	web, _ := svc.DB.GetByProjectName("krayon", "web")
	v1, _ := svc.DB.GetByProjectName("krayon", "v1")
	if web.Status != registry.StatusStopped {
		t.Fatalf("shadow %+v", web)
	}
	if v1.Status != registry.StatusRunning {
		t.Fatalf("active %+v", v1)
	}
	if ShouldRestart(web, "on-failure") {
		t.Fatal("stopped must not restart")
	}
}

func TestShouldRestartIgnoresStopped(t *testing.T) {
	if ShouldRestart(registry.Session{Status: registry.StatusStopped}, "always") {
		t.Fatal()
	}
	if !ShouldRestart(registry.Session{Status: registry.StatusExited}, "on-failure") {
		t.Fatal()
	}
}

func TestDownAllStopsActives(t *testing.T) {
	svc := testProfileService(t)
	if _, err := svc.NewActive("krayon", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := Down(svc, "krayon", true); err != nil {
		t.Fatal(err)
	}
	v1, _ := svc.DB.GetByProjectName("krayon", "v1")
	if v1.Status != registry.StatusStopped {
		t.Fatalf("%+v", v1)
	}
}

func TestProfileCommandShellArgv(t *testing.T) {
	svc := testProfileService(t)
	dir := svc.Cfg.Projects["krayon"]
	cmd := "echo a; echo b"
	yaml := "name: krayon\nshadows:\n  - name: q\n    command: " + strconv.Quote(cmd) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "devhub.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	pr, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	pr.Path = dir
	if _, err := Up(svc, pr, false); err != nil {
		t.Fatal(err)
	}
	sess, err := svc.DB.GetByProjectName("krayon", "q")
	if err != nil {
		t.Fatal(err)
	}
	argv := core.ParseArgv(sess)
	if len(argv) != 3 || argv[2] != cmd {
		t.Fatalf("%v", argv)
	}
}

func testProfileService(t *testing.T) *core.Service {
	t.Helper()
	db, err := registry.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cfg := config.Default()
	cfg.Projects = map[string]string{"krayon": t.TempDir()}
	return &core.Service{Cfg: cfg, Tmux: tmux.NewFake(), DB: db, Sampler: monitor.NewSampler(nil)}
}
