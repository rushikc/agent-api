package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rushikesh/devhub/internal/alerts"
	"github.com/rushikesh/devhub/internal/api"
	"github.com/rushikesh/devhub/internal/client"
	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/e2e"
	"github.com/rushikesh/devhub/internal/events"
	"github.com/rushikesh/devhub/internal/gcp"
	"github.com/rushikesh/devhub/internal/monitor"
	"github.com/rushikesh/devhub/internal/profile"
	"github.com/rushikesh/devhub/internal/secrets"
	"github.com/rushikesh/devhub/internal/skills"
)

func Run(cfg *config.Config, svc *core.Service) error {
	if err := os.MkdirAll(cfg.LogDir(), 0o755); err != nil {
		return err
	}
	logf, err := openRotating(cfg.DaemonLogPath())
	if err != nil {
		return err
	}
	defer logf.Close()
	_, _ = fmt.Fprintf(logf, "daemon start %s\n", time.Now().Format(time.RFC3339))

	if err := os.WriteFile(cfg.PidPath(), []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		return err
	}
	defer os.Remove(cfg.PidPath())

	hub, _ := skills.Open(cfg)
	bus := events.NewBus()
	gw := gcp.NewGateway(cfg, svc.DB, bus)
	sec := secrets.New(gw)
	eng, _ := e2e.NewDocker()
	if eng == nil {
		eng = e2e.NewFake()
	}
	eq := e2e.NewQueue(cfg, svc.DB, eng, bus, sec)
	al := alerts.New(cfg.Alerts, svc.DB)
	al.Subscribe(bus)
	svc.Publish = func(kind, target, project string, payload map[string]any) {
		bus.Publish(events.Event{Type: kind, Target: target, Project: project, Payload: payload})
	}
	svc.ResolveEnv = func(project string, extra map[string]string) map[string]string {
		return sec.ResolveMap(context.Background(), extra)
	}
	svc.ProfileEnv = func(project string) map[string]string {
		return profile.EnvForProject(cfg, project)
	}

	srv := api.NewWith(cfg, svc, hub, api.Deps{GCP: gw, E2E: eq, Secrets: sec, Alerts: al, Bus: bus})
	tok, _ := api.EnsureToken(cfg)
	srv.Token = tok

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer sec.Lock()

	lock, lockErr := monitor.TryLock(cfg.MonitorLockPath())
	if lockErr == nil {
		defer lock.Close()
	}
	go func() {
		_ = monitor.RunForeverHook(svc.Tmux, svc.DB, cfg.StatusPath(), 2*time.Second, ctx.Done(), func(snap monitor.Snapshot) {
			al.ObserveSnapshot(bus, snap)
		})
	}()
	go profile.Supervise(ctx, svc)
	if hub != nil {
		go watchSkills(ctx, hub)
	}
	go watchConfig(ctx, cfg, svc)
	go handleSIGHUP(ctx, cfg, svc)
	_ = svc.ApplyKeybindingsForce(true)
	errCh := make(chan error, 2)
	go func() { errCh <- api.ServeUnix(ctx, srv, cfg.SocketPath()) }()
	if cfg.API.EnableTCP {
		tcp := srv
		tcp.RequireToken = true
		addr := fmt.Sprintf("127.0.0.1:%d", cfg.API.TCPPort)
		go func() { errCh <- api.ServeTCP(ctx, tcp, addr) }()
	}
	return <-errCh
}

func handleSIGHUP(ctx context.Context, cfg *config.Config, svc *core.Service) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			_ = cfg.Reload()
			_ = svc.ApplyKeybindingsForce(true)
		}
	}
}

func watchConfig(ctx context.Context, cfg *config.Config, svc *core.Service) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer w.Close()
	_ = w.Add(cfg.Dir())
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-w.Events:
			if filepath.Base(ev.Name) != config.ConfigName && ev.Name != cfg.Dir() {
				if !strings.HasSuffix(ev.Name, config.ConfigName) {
					continue
				}
			}
			timer.Reset(300 * time.Millisecond)
		case <-timer.C:
			_ = cfg.Reload()
			_ = svc.ApplyKeybindingsForce(true)
		case <-w.Errors:
		}
	}
}

func watchSkills(ctx context.Context, hub *skills.Hub) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer w.Close()
	_ = w.Add(hub.Root)
	_ = w.Add(filepath.Join(hub.Root, "skills"))
	_ = w.Add(filepath.Join(hub.Root, "rules"))
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.Events:
			timer.Reset(500 * time.Millisecond)
		case <-timer.C:
			_, _ = hub.Sync(false)
		case <-w.Errors:
		}
	}
}

func Ensure(cfg *config.Config) error {
	c := client.Unix(cfg, "cli")
	if c.Healthy() {
		return nil
	}
	exe := "devhub"
	if p, err := os.Executable(); err == nil {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		if filepath.Base(p) == "dev" {
			sib := filepath.Join(filepath.Dir(p), "devhub")
			if _, err := os.Stat(sib); err == nil {
				p = sib
			}
		}
		exe = p
	}
	cmd := exec.Command(exe, "daemon")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if client.Unix(cfg, "cli").Healthy() {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not become healthy")
}

func Status(cfg *config.Config) string {
	b, _ := os.ReadFile(cfg.PidPath())
	pid := strings.TrimSpace(string(b))
	ok := client.Unix(cfg, "cli").Healthy()
	if ok {
		return fmt.Sprintf("running pid=%s socket=%s", pid, cfg.SocketPath())
	}
	return "stopped"
}

func Stop(cfg *config.Config) error {
	b, err := os.ReadFile(cfg.PidPath())
	if err != nil {
		return nil
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if pid <= 1 {
		return nil
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	_ = p.Signal(syscall.SIGTERM)
	time.Sleep(200 * time.Millisecond)
	_ = os.Remove(cfg.SocketPath())
	_ = os.Remove(cfg.PidPath())
	return nil
}

func Logs(cfg *config.Config, n int) (string, error) {
	b, err := os.ReadFile(cfg.DaemonLogPath())
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(b), "\n")
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n"), nil
}

func InstallLaunchd(cfg *config.Config) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	plist := filepath.Join(dir, "dev.devhub.daemon.plist")
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>dev.devhub.daemon</string>
  <key>ProgramArguments</key>
  <array><string>%s</string><string>daemon</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, exe, cfg.DaemonLogPath(), cfg.DaemonLogPath())
	if err := os.WriteFile(plist, []byte(body), 0o644); err != nil {
		return "", err
	}
	_ = exec.Command("launchctl", "load", plist).Run()
	return plist, nil
}

func UninstallLaunchd() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	plist := filepath.Join(home, "Library", "LaunchAgents", "dev.devhub.daemon.plist")
	_ = exec.Command("launchctl", "unload", plist).Run()
	return os.Remove(plist)
}

func openRotating(path string) (*os.File, error) {
	if st, err := os.Stat(path); err == nil && st.Size() > 5*1024*1024 {
		_ = os.Rename(path+".1", path+".2")
		_ = os.Rename(path, path+".1")
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}
