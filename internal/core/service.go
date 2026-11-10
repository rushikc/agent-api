package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/monitor"
	"github.com/rushikesh/devhub/internal/registry"
	"github.com/rushikesh/devhub/internal/tmux"
)

// Service is the session API used by the CLI, TUI, and (in v2) an HTTP layer.
type Service struct {
	Cfg     *config.Config
	Tmux    tmux.Client
	DB      *registry.DB
	Sampler *monitor.Sampler
	exe     string
}

func Open() (*Service, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	db, err := registry.Open(cfg.DBPath())
	if err != nil {
		return nil, err
	}
	cli := tmux.New()
	return &Service{
		Cfg:     cfg,
		Tmux:    cli,
		DB:      db,
		Sampler: monitor.NewSampler(nil),
	}, nil
}

func (s *Service) Close() error {
	if s.DB != nil {
		return s.DB.Close()
	}
	return nil
}

func (s *Service) Executable() string {
	if s.exe != "" {
		return s.exe
	}
	exe, err := os.Executable()
	if err != nil {
		return "devhub"
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if filepath.Base(exe) == "dev" {
		sibling := filepath.Join(filepath.Dir(exe), "devhub")
		if _, err := os.Stat(sibling); err == nil {
			exe = sibling
		}
	}
	s.exe = exe
	return exe
}

func (s *Service) projectEnv(project string) map[string]string {
	return map[string]string{
		"DEVHUB_PROJECT": project,
		"DEVHUB_COLOR":   s.Cfg.ColorFor(project),
	}
}

func (s *Service) decorateSession(session, project, cwd string) error {
	env := s.projectEnv(project)
	for k, v := range env {
		if err := s.Tmux.SetEnvironment(session, k, v); err != nil {
			return err
		}
	}
	if err := s.Tmux.SetDefaultPath(session, cwd); err != nil {
		return err
	}
	if err := s.Tmux.SetUserOption(session, "@project", project); err != nil {
		return err
	}
	return s.Tmux.SetUserOption(session, "@color", env["DEVHUB_COLOR"])
}

func (s *Service) Reconcile() error {
	panes, err := s.Tmux.ListPanes()
	if err != nil {
		return err
	}
	return s.DB.Reconcile(panes)
}

type SessionView struct {
	Session  registry.Session
	CPUPct   float64
	RSSBytes uint64
	PIDs     []int32
}

func (s *Service) List(all bool) ([]SessionView, error) {
	if err := s.Reconcile(); err != nil {
		return nil, err
	}
	rows, err := s.DB.List(all)
	if err != nil {
		return nil, err
	}
	out := make([]SessionView, 0, len(rows))
	for _, r := range rows {
		v := SessionView{Session: r}
		if r.Status == registry.StatusRunning && r.PanePID > 0 {
			sample := s.Sampler.Sample(int32(r.PanePID))
			v.CPUPct = sample.CPUPct
			v.RSSBytes = sample.RSSBytes
			v.PIDs = sample.PIDs
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) Lookup(ref string) (registry.Session, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return registry.Session{}, fmt.Errorf("name is required")
	}
	if project, name, ok := strings.Cut(ref, "/"); ok && project != "" && name != "" {
		sess, err := s.DB.GetByProjectName(project, name)
		if err != nil {
			return registry.Session{}, fmt.Errorf("no session %s", ref)
		}
		return sess, nil
	}
	found, err := s.DB.FindByName(ref)
	if err != nil {
		return registry.Session{}, err
	}
	if len(found) == 0 {
		return registry.Session{}, fmt.Errorf("no session named %q", ref)
	}
	if len(found) > 1 {
		var opts []string
		for _, f := range found {
			opts = append(opts, f.Project+"/"+f.Name)
		}
		return registry.Session{}, fmt.Errorf("ambiguous name %q; use one of: %s", ref, strings.Join(opts, ", "))
	}
	return found[0], nil
}

func (s *Service) NewActive(project, name string) (registry.Session, error) {
	project, err := s.resolveProject(project)
	if err != nil {
		return registry.Session{}, err
	}
	cwd, err := s.Cfg.ProjectPath(project)
	if err != nil {
		return registry.Session{}, err
	}
	if name == "" {
		name, err = s.nextActiveName(project)
		if err != nil {
			return registry.Session{}, err
		}
	}
	existing, existingErr := s.DB.GetByProjectName(project, name)
	if existingErr == nil && existing.Status == registry.StatusRunning {
		return existing, nil
	}
	created, err := s.ensureProjectSession(project, cwd, name)
	if err != nil {
		return registry.Session{}, err
	}
	sessName := tmux.ProjectSession(project)
	if !created && !windowExists(s.Tmux, sessName, name) {
		if err := s.Tmux.NewWindow(tmux.WindowOpts{
			Session: sessName, Name: name, Cwd: cwd, Env: s.projectEnv(project),
		}); err != nil {
			return registry.Session{}, err
		}
	}
	target, pid, err := s.paneInfo(sessName, name)
	if err != nil {
		return registry.Session{}, err
	}
	row := registry.Session{
		Project:    project,
		Name:       name,
		Kind:       registry.KindActive,
		Cwd:        cwd,
		TmuxTarget: target,
		PanePID:    pid,
		CreatedAt:  time.Now(),
		Status:     registry.StatusRunning,
	}
	if existingErr == nil && existing.ID != 0 {
		row.ID = existing.ID
		if err := s.DB.Update(row); err != nil {
			return registry.Session{}, err
		}
		_ = s.EnsureMonitor()
		return row, nil
	}
	id, err := s.DB.Insert(row)
	if err != nil {
		return registry.Session{}, err
	}
	row.ID = id
	_ = s.EnsureMonitor()
	return row, nil
}

func windowExists(client tmux.Client, session, window string) bool {
	panes, err := client.ListPanes()
	if err != nil {
		return false
	}
	for _, p := range panes {
		if p.Session == session && p.Window == window && !p.Dead {
			return true
		}
	}
	return false
}

func (s *Service) ensureProjectSession(project, cwd, firstWindow string) (created bool, err error) {
	sess := tmux.ProjectSession(project)
	has, err := s.Tmux.HasSession(sess)
	if err != nil {
		return false, err
	}
	if has {
		return false, nil
	}
	if firstWindow == "" {
		firstWindow = "v1"
	}
	if err := s.Tmux.NewSession(tmux.SessionOpts{
		Name: sess, Window: firstWindow, Cwd: cwd, Env: s.projectEnv(project),
	}); err != nil {
		return false, err
	}
	if err := s.decorateSession(sess, project, cwd); err != nil {
		return true, err
	}
	return true, nil
}

func (s *Service) nextActiveName(project string) (string, error) {
	names, err := s.DB.NamesInProject(project, registry.KindActive)
	if err != nil {
		return "", err
	}
	used := map[string]bool{}
	for _, n := range names {
		used[n] = true
	}
	for i := 1; i < 1000; i++ {
		n := fmt.Sprintf("v%d", i)
		if !used[n] {
			return n, nil
		}
	}
	return "", fmt.Errorf("too many terminals in %s", project)
}

func (s *Service) NewShadow(project, name string, command []string) (registry.Session, error) {
	if project == "" {
		return registry.Session{}, fmt.Errorf("project is required")
	}
	if len(command) == 0 {
		return registry.Session{}, fmt.Errorf("command is required (example: devhub shadow %s -- npm run dev)", project)
	}
	cwd, err := s.Cfg.ProjectPath(project)
	if err != nil {
		return registry.Session{}, err
	}
	if name == "" {
		name, err = s.uniqueShadowName(project, sanitizeName(command))
		if err != nil {
			return registry.Session{}, err
		}
	}
	sess := tmux.ShadowSession(project)
	has, err := s.Tmux.HasSession(sess)
	if err != nil {
		return registry.Session{}, err
	}
	env := s.projectEnv(project)
	optsWin := tmux.WindowOpts{Session: sess, Name: name, Cwd: cwd, Command: command, Env: env, RemainOnExit: true}
	if !has {
		if err := s.Tmux.NewSession(tmux.SessionOpts{
			Name: sess, Window: name, Cwd: cwd, Command: command, Env: env, RemainOnExit: true,
		}); err != nil {
			return registry.Session{}, err
		}
		if err := s.decorateSession(sess, project, cwd); err != nil {
			return registry.Session{}, err
		}
	} else {
		if err := s.Tmux.NewWindow(optsWin); err != nil {
			return registry.Session{}, err
		}
	}
	target, pid, err := s.paneInfo(sess, name)
	if err != nil {
		return registry.Session{}, err
	}
	row := registry.Session{
		Project:    project,
		Name:       name,
		Kind:       registry.KindShadow,
		Command:    strings.Join(command, " "),
		Cwd:        cwd,
		TmuxTarget: target,
		PanePID:    pid,
		CreatedAt:  time.Now(),
		Status:     registry.StatusRunning,
	}
	id, err := s.DB.Insert(row)
	if err != nil {
		return registry.Session{}, err
	}
	row.ID = id
	_ = s.EnsureMonitor()
	return row, nil
}

func (s *Service) uniqueShadowName(project, base string) (string, error) {
	if base == "" {
		base = "shadow"
	}
	names, err := s.DB.NamesInProject(project, registry.KindShadow)
	if err != nil {
		return "", err
	}
	used := map[string]bool{}
	for _, n := range names {
		used[n] = true
	}
	if !used[base] {
		return base, nil
	}
	for i := 2; i < 1000; i++ {
		n := fmt.Sprintf("%s-%d", base, i)
		if !used[n] {
			return n, nil
		}
	}
	return "", fmt.Errorf("could not name shadow")
}

func (s *Service) Peek(ref string) (string, error) {
	sess, err := s.Lookup(ref)
	if err != nil {
		return "", err
	}
	if sess.TmuxTarget == "" {
		return "", fmt.Errorf("%s has no tmux pane", ref)
	}
	return s.Tmux.CapturePane(sess.TmuxTarget, -200)
}

func (s *Service) Attach(ref string) error {
	sess, err := s.Lookup(ref)
	if err != nil {
		return err
	}
	return s.Tmux.SwitchClient(sess.TmuxTarget)
}

func (s *Service) Kill(ref string) error {
	sess, err := s.Lookup(ref)
	if err != nil {
		return err
	}
	if sess.TmuxTarget != "" {
		_ = s.Tmux.KillWindow(sess.TmuxTarget)
	}
	sess.Status = registry.StatusExited
	sess.PanePID = 0
	return s.DB.Update(sess)
}

func (s *Service) Restart(ref string) error {
	sess, err := s.Lookup(ref)
	if err != nil {
		return err
	}
	if strings.TrimSpace(sess.Command) == "" {
		return fmt.Errorf("%s has no stored command to restart", ref)
	}
	cmd := strings.Fields(sess.Command)
	cwd := sess.Cwd
	if p, err := s.Cfg.ProjectPath(sess.Project); err == nil {
		cwd = p
	}
	if cwd == "" {
		return fmt.Errorf("no cwd for %s", ref)
	}
	env := s.projectEnv(sess.Project)
	var sessionName string
	remain := false
	if sess.Kind == registry.KindShadow {
		sessionName = tmux.ShadowSession(sess.Project)
		remain = true
	} else {
		sessionName = tmux.ProjectSession(sess.Project)
	}
	if sess.TmuxTarget != "" {
		if ok, _ := s.Tmux.HasPane(sess.TmuxTarget); ok {
			if err := s.Tmux.RespawnPane(tmux.RespawnOpts{
				Target: sess.TmuxTarget, Cwd: cwd, Command: cmd, Env: env,
			}); err != nil {
				return err
			}
			target, pid, err := s.paneInfo(sessionName, sess.Name)
			if err != nil {
				return err
			}
			sess.TmuxTarget = target
			sess.PanePID = pid
			sess.Status = registry.StatusRunning
			sess.ExitCode = nil
			sess.CreatedAt = time.Now()
			return s.DB.Update(sess)
		}
	}
	has, err := s.Tmux.HasSession(sessionName)
	if err != nil {
		return err
	}
	if !has {
		if err := s.Tmux.NewSession(tmux.SessionOpts{
			Name: sessionName, Window: sess.Name, Cwd: cwd, Command: cmd, Env: env, RemainOnExit: remain,
		}); err != nil {
			return err
		}
		_ = s.decorateSession(sessionName, sess.Project, cwd)
	} else {
		if err := s.Tmux.NewWindow(tmux.WindowOpts{
			Session: sessionName, Name: sess.Name, Cwd: cwd, Command: cmd, Env: env, RemainOnExit: remain,
		}); err != nil {
			return err
		}
	}
	target, pid, err := s.paneInfo(sessionName, sess.Name)
	if err != nil {
		return err
	}
	sess.TmuxTarget = target
	sess.PanePID = pid
	sess.Status = registry.StatusRunning
	sess.ExitCode = nil
	sess.CreatedAt = time.Now()
	return s.DB.Update(sess)
}

func (s *Service) Promote(ref string) error {
	sess, err := s.Lookup(ref)
	if err != nil {
		return err
	}
	if sess.Kind != registry.KindShadow {
		return fmt.Errorf("%s is not a shadow terminal", ref)
	}
	cwd, err := s.Cfg.ProjectPath(sess.Project)
	if err != nil {
		return err
	}
	if _, err := s.ensureProjectSession(sess.Project, cwd, ""); err != nil {
		return err
	}
	dst := tmux.ProjectSession(sess.Project)
	if err := s.Tmux.MoveWindow(sess.TmuxTarget, dst); err != nil {
		return err
	}
	sess.Kind = registry.KindActive
	sess.TmuxTarget = dst + ":" + sess.Name
	return s.DB.Update(sess)
}

func (s *Service) Rename(ref, newName string) error {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return fmt.Errorf("new name is required")
	}
	sess, err := s.Lookup(ref)
	if err != nil {
		return err
	}
	if sess.TmuxTarget != "" {
		if err := s.Tmux.RenameWindow(sess.TmuxTarget, newName); err != nil {
			return err
		}
	}
	oldName := sess.Name
	sess.Name = newName
	sess.TmuxTarget = strings.TrimSuffix(sess.TmuxTarget, ":"+oldName) + ":" + newName
	return s.DB.Update(sess)
}

func (s *Service) Jump(n int, session string) error {
	if n < 1 || n > 9 {
		return fmt.Errorf("jump target must be 1-9")
	}
	if session == "" {
		var err error
		session, err = s.Tmux.CurrentSession()
		if err != nil {
			return err
		}
	}
	project, ok := tmux.ProjectFromSession(session)
	if !ok {
		return fmt.Errorf("not in a project session")
	}
	name := fmt.Sprintf("v%d", n)
	row, err := s.DB.GetByProjectName(project, name)
	if err != nil {
		return fmt.Errorf("no %s in %s", name, project)
	}
	return s.Tmux.SwitchClient(row.TmuxTarget)
}

func (s *Service) SwitchProject(name string) error {
	cwd, err := s.Cfg.ProjectPath(name)
	if err != nil {
		return err
	}
	sess := tmux.ProjectSession(name)
	created, err := s.ensureProjectSession(name, cwd, "v1")
	if err != nil {
		return err
	}
	if !created {
		if err := s.decorateSession(sess, name, cwd); err != nil {
			return err
		}
	}
	if _, err := s.DB.GetByProjectName(name, "v1"); err != nil {
		target, pid, perr := s.paneInfo(sess, "v1")
		if perr == nil {
			_, _ = s.DB.Insert(registry.Session{
				Project: name, Name: "v1", Kind: registry.KindActive, Cwd: cwd,
				TmuxTarget: target, PanePID: pid, CreatedAt: time.Now(), Status: registry.StatusRunning,
			})
		}
	}
	_ = s.EnsureMonitor()
	return s.Tmux.SwitchClient(sess)
}

func (s *Service) EnsureMain() error {
	if err := s.Tmux.EnsureServer(); err != nil {
		return err
	}
	has, err := s.Tmux.HasSession(tmux.MainSession)
	if err != nil {
		return err
	}
	if !has {
		home, _ := os.UserHomeDir()
		if home == "" {
			home = "."
		}
		if err := s.Tmux.NewSession(tmux.SessionOpts{Name: tmux.MainSession, Window: tmux.MainWindow, Cwd: home}); err != nil {
			return err
		}
	}
	if err := s.ApplyKeybindings(); err != nil {
		return err
	}
	if err := s.EnsureMonitor(); err != nil {
		return err
	}
	return s.Tmux.SwitchClient(tmux.MainSession)
}

func (s *Service) ApplyKeybindings() error {
	path := s.Cfg.TmuxConfPath()
	body := RenderTmuxConf(s.Cfg, s.Executable())
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}
	return s.Tmux.SourceFile(path)
}

func (s *Service) EnsureMonitor() error {
	if strings.HasSuffix(filepath.Base(s.Executable()), ".test") {
		return nil
	}
	lock, err := monitor.TryLock(s.Cfg.MonitorLockPath())
	if err != nil {
		// another monitor holds the lock
		return nil
	}
	_ = lock.Close() // release so the child can take it
	exe := s.Executable()
	cmd := exec.Command(exe, "monitor")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	return cmd.Start()
}

func (s *Service) paneInfo(session, window string) (target string, pid int, err error) {
	target = session + ":" + window
	panes, err := s.Tmux.ListPanes()
	if err != nil {
		return target, 0, err
	}
	for _, p := range panes {
		if p.Session == session && p.Window == window {
			return p.Target, p.PID, nil
		}
	}
	return target, 0, nil
}

func (s *Service) resolveProject(project string) (string, error) {
	if project != "" {
		if _, err := s.Cfg.ProjectPath(project); err != nil {
			return "", err
		}
		return project, nil
	}
	cur, err := s.Tmux.CurrentSession()
	if err == nil {
		if p, ok := tmux.ProjectFromSession(cur); ok {
			return p, nil
		}
	}
	if len(s.Cfg.Projects) == 1 {
		for name := range s.Cfg.Projects {
			return name, nil
		}
	}
	if len(s.Cfg.Projects) == 0 {
		return "", fmt.Errorf("no projects configured (devhub project add <name> <path>)")
	}
	return "", fmt.Errorf("specify --project (one of: %s)", strings.Join(projectNames(s.Cfg), ", "))
}

func projectNames(cfg *config.Config) []string {
	var names []string
	for n := range cfg.Projects {
		names = append(names, n)
	}
	return names
}

var nonName = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func sanitizeName(cmd []string) string {
	base := strings.Join(cmd, "-")
	base = nonName.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if len(base) > 32 {
		base = base[:32]
		base = strings.Trim(base, "-")
	}
	if base == "" {
		return "shadow"
	}
	return strings.ToLower(base)
}

func ParseJumpN(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid jump %q", s)
	}
	return n, nil
}

func Quote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func RenderTmuxConf(cfg *config.Config, exe string) string {
	prefix := config.DefaultPrefix
	mode := "both"
	if cfg != nil {
		if cfg.Prefix != "" {
			prefix = cfg.Prefix
		}
		mode = cfg.KeysMode()
	}
	exeQ := Quote(exe)
	var b strings.Builder
	b.WriteString("# Generated by devhub. Do not edit.\n")
	b.WriteString("# Loaded via: tmux source-file ~/.devhub/tmux.conf\n\n")
	b.WriteString("unbind-key C-b\n")
	fmt.Fprintf(&b, "set-option -g prefix %s\n", prefix)
	fmt.Fprintf(&b, "bind-key %s send-prefix\n\n", prefix)
	b.WriteString("set-option -g default-terminal tmux-256color\n")
	b.WriteString("set-option -g status-interval 2\n")
	b.WriteString("set-option -g status-left-length 40\n")
	b.WriteString("set-option -g status-right-length 80\n")
	b.WriteString("set-option -g status-left '#[bold,fg=black,bg=#{@color}] #{@project} #[default] '\n")
	fmt.Fprintf(&b, "set-option -g status-right '#(%s status --short)'\n", exeQ)
	b.WriteString("set-option -g window-status-format '#[fg=#{@color}]#{@project}:#W#[default]'\n")
	b.WriteString("set-option -g window-status-current-format '#[bold,fg=black,bg=#{@color}] #{@project}:#W #[default]'\n\n")

	writePopup := func(key, ui, size string, prefixBind bool) {
		cmd := Quote(exe + " " + ui)
		if prefixBind {
			fmt.Fprintf(&b, "bind-key %s display-popup -E %s %s\n", key, size, cmd)
		} else {
			fmt.Fprintf(&b, "bind-key -n M-%s display-popup -E %s %s\n", key, size, cmd)
		}
	}
	writeJump := func(n int, prefixBind bool) {
		cmd := Quote(exe + " jump " + strconv.Itoa(n) + " --session #{session_name}")
		if prefixBind {
			fmt.Fprintf(&b, "bind-key %d run-shell %s\n", n, cmd)
		} else {
			fmt.Fprintf(&b, "bind-key -n M-%d run-shell %s\n", n, cmd)
		}
	}

	usePrefix := mode == "prefix" || mode == "both"
	useOption := mode == "option" || mode == "both"
	if useOption {
		b.WriteString("# Option/Meta (no prefix). Do not bind M-b / M-f / M-d (readline).\n")
		writePopup("s", "picker", "-w 80% -h 70%", false)
		writePopup("h", "shadows", "-w 80% -h 70%", false)
		writePopup("r", "top", "-w 90% -h 80%", false)
		writePopup("p", "projects", "-w 50% -h 50%", false)
		for i := 1; i <= 9; i++ {
			writeJump(i, false)
		}
		b.WriteString("# v2: M-k skills, M-o ports/orphans, M-u up/down\n\n")
	}
	if usePrefix {
		b.WriteString("# Prefix bindings (Ctrl-Space by default)\n")
		writePopup("s", "picker", "-w 80% -h 70%", true)
		writePopup("h", "shadows", "-w 80% -h 70%", true)
		writePopup("r", "top", "-w 90% -h 80%", true)
		writePopup("p", "projects", "-w 50% -h 50%", true)
		b.WriteString("\n")
		for i := 1; i <= 9; i++ {
			writeJump(i, true)
		}
	}
	return b.String()
}
