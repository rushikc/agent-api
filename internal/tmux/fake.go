package tmux

import (
	"fmt"
	"strings"
	"sync"
)

type Call struct {
	Op      string
	Session string
	Window  string
	Cwd     string
	Env     map[string]string
	Target  string
	Command []string
}

// Fake is an in-memory tmux client for tests.
type Fake struct {
	mu        sync.Mutex
	VersionS  string
	Sessions  map[string]*fakeSession
	Current   string
	Sourced   []string
	Fail      error
	NoServer  bool
	Calls     []Call
	nextID    int
	OmitPanes bool
}

type fakeSession struct {
	Name     string
	Attached bool
	Windows  []*fakeWindow
}

type fakeWindow struct {
	ID           string
	Name         string
	Cwd          string
	Command      []string
	PID          int
	Dead         bool
	DeadStatus   int
	RemainOnExit bool
	Killed       bool
	Output       string
}

func NewFake() *Fake {
	return &Fake{
		VersionS: "tmux 3.4",
		Sessions: map[string]*fakeSession{},
	}
}

func (f *Fake) Version() (string, error) {
	if f.Fail != nil {
		return "", f.Fail
	}
	return f.VersionS, nil
}

func (f *Fake) EnsureServer() error { return f.Fail }

func (f *Fake) HasSession(name string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return false, f.Fail
	}
	_, ok := f.Sessions[name]
	return ok, nil
}

func (f *Fake) record(c Call) {
	f.Calls = append(f.Calls, c)
}

func (f *Fake) NewSession(opts SessionOpts) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return f.Fail
	}
	if strings.TrimSpace(opts.Cwd) == "" {
		return fmt.Errorf("cwd is required")
	}
	f.record(Call{Op: "new-session", Session: opts.Name, Window: opts.Window, Cwd: opts.Cwd, Env: opts.Env, Command: opts.Command})
	if _, ok := f.Sessions[opts.Name]; ok {
		return fmt.Errorf("duplicate session %s", opts.Name)
	}
	win := opts.Window
	if win == "" {
		win = "0"
	}
	f.Sessions[opts.Name] = &fakeSession{
		Name: opts.Name,
		Windows: []*fakeWindow{{
			ID:           f.allocID(),
			Name:         win,
			Cwd:          opts.Cwd,
			Command:      opts.Command,
			PID:          nextPID(opts.Name, win),
			RemainOnExit: opts.RemainOnExit,
		}},
	}
	return nil
}

func (f *Fake) NewWindow(opts WindowOpts) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return f.Fail
	}
	if strings.TrimSpace(opts.Cwd) == "" {
		return fmt.Errorf("cwd is required")
	}
	f.record(Call{Op: "new-window", Session: opts.Session, Window: opts.Name, Cwd: opts.Cwd, Env: opts.Env, Command: opts.Command})
	s, ok := f.Sessions[opts.Session]
	if !ok {
		return fmt.Errorf("no session %s", opts.Session)
	}
	s.Windows = append(s.Windows, &fakeWindow{
		ID:           f.allocID(),
		Name:         opts.Name,
		Cwd:          opts.Cwd,
		Command:      opts.Command,
		PID:          nextPID(opts.Session, opts.Name),
		RemainOnExit: opts.RemainOnExit,
	})
	return nil
}

func (f *Fake) ListPanes() ([]Pane, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return nil, f.Fail
	}
	if f.OmitPanes {
		return nil, nil
	}
	var out []Pane
	for _, s := range f.Sessions {
		for i, w := range s.Windows {
			if w.Killed {
				continue
			}
			id := w.ID
			if id == "" {
				id = fmt.Sprintf("@%d", i)
			}
			out = append(out, Pane{
				Session:    s.Name,
				Window:     w.Name,
				WindowID:   id,
				PaneID:     fmt.Sprintf("%%%s-%d", s.Name, i),
				PID:        w.PID,
				Dead:       w.Dead,
				DeadStatus: w.DeadStatus,
				Path:       w.Cwd,
				Target:     s.Name + ":" + id,
			})
		}
	}
	return out, nil
}

func (f *Fake) ListSessions() ([]Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return nil, f.Fail
	}
	var out []Session
	for _, s := range f.Sessions {
		out = append(out, Session{Name: s.Name, Attached: s.Attached})
	}
	return out, nil
}

func (f *Fake) SetPaneOutput(target, s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w := f.window(target)
	if w != nil {
		w.Output = s
	}
}

func (f *Fake) CapturePane(target string, start int) (string, error) {
	_ = start
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return "", f.Fail
	}
	w := f.window(target)
	if w == nil {
		return "", fmt.Errorf("no pane %s", target)
	}
	if w.Output != "" {
		return w.Output, nil
	}
	return strings.Join(w.Command, " ") + " output\n", nil
}

func (f *Fake) SwitchClient(target string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return f.Fail
	}
	f.record(Call{Op: "switch-client", Target: target})
	sess, _, _ := strings.Cut(target, ":")
	s, ok := f.Sessions[sess]
	if !ok {
		return fmt.Errorf("no session %s", sess)
	}
	s.Attached = true
	f.Current = target
	return nil
}

func (f *Fake) KillWindow(target string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return f.Fail
	}
	sess, name, ok := strings.Cut(target, ":")
	if !ok {
		return fmt.Errorf("bad target")
	}
	s, ok := f.Sessions[sess]
	if !ok {
		return fmt.Errorf("no session")
	}
	for _, w := range s.Windows {
		if windowMatches(w, name) {
			w.Killed = true
			f.record(Call{Op: "kill-window", Target: target, Session: sess, Window: w.Name})
			return nil
		}
	}
	return fmt.Errorf("no window %s", target)
}

func (f *Fake) RenameWindow(target, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return f.Fail
	}
	w := f.window(target)
	if w == nil {
		return fmt.Errorf("no window %s", target)
	}
	w.Name = name
	return nil
}

func (f *Fake) MoveWindow(src, dstSession string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return f.Fail
	}
	dstSession = strings.TrimSuffix(dstSession, ":")
	sess, name, ok := strings.Cut(src, ":")
	if !ok {
		return fmt.Errorf("bad src")
	}
	from, ok := f.Sessions[sess]
	if !ok {
		return fmt.Errorf("no session")
	}
	to, ok := f.Sessions[dstSession]
	if !ok {
		return fmt.Errorf("no dest session")
	}
	var w *fakeWindow
	var rest []*fakeWindow
	for _, win := range from.Windows {
		if windowMatches(win, name) && !win.Killed {
			w = win
			continue
		}
		rest = append(rest, win)
	}
	if w == nil {
		return fmt.Errorf("no window")
	}
	from.Windows = rest
	to.Windows = append(to.Windows, w)
	return nil
}

func (f *Fake) SourceFile(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.NoServer {
		return ErrNoServer
	}
	if f.Fail != nil {
		return f.Fail
	}
	f.Sourced = append(f.Sourced, path)
	return nil
}

func (f *Fake) CurrentSession() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return "", f.Fail
	}
	if f.Current != "" {
		sess, _, _ := strings.Cut(f.Current, ":")
		return sess, nil
	}
	return MainSession, nil
}

func (f *Fake) SetRemainOnExit(target string, on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return f.Fail
	}
	w := f.window(target)
	if w == nil {
		return fmt.Errorf("no window %s", target)
	}
	w.RemainOnExit = on
	return nil
}

func (f *Fake) SetEnvironment(target, key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return f.Fail
	}
	f.record(Call{Op: "set-environment", Target: target, Env: map[string]string{key: value}})
	return nil
}

func (f *Fake) SetUserOption(target, key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return f.Fail
	}
	f.record(Call{Op: "set-option", Target: target, Env: map[string]string{key: value}})
	return nil
}

func (f *Fake) RespawnPane(opts RespawnOpts) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return f.Fail
	}
	if strings.TrimSpace(opts.Cwd) == "" {
		return fmt.Errorf("cwd is required")
	}
	f.record(Call{Op: "respawn-pane", Target: opts.Target, Cwd: opts.Cwd, Env: opts.Env, Command: opts.Command})
	w := f.window(opts.Target)
	if w == nil {
		return fmt.Errorf("no pane %s", opts.Target)
	}
	w.Cwd = opts.Cwd
	w.Command = opts.Command
	w.Dead = false
	w.Killed = false
	w.PID = nextPID(opts.Target, w.Name)
	return nil
}

func (f *Fake) SendKeys(target, keys string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return f.Fail
	}
	f.record(Call{Op: "send-keys", Target: target, Command: []string{keys}})
	if f.window(target) == nil {
		return fmt.Errorf("no pane %s", target)
	}
	return nil
}

func (f *Fake) HasPane(target string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return false, f.Fail
	}
	return f.window(target) != nil, nil
}

func (f *Fake) MarkDead(target string, exitCode int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w := f.window(target)
	if w != nil {
		w.Dead = true
		w.DeadStatus = exitCode
		w.PID = 0
	}
}

func (f *Fake) allocID() string {
	f.nextID++
	return fmt.Sprintf("@%d", f.nextID)
}

func windowMatches(w *fakeWindow, name string) bool {
	if w == nil || w.Killed {
		return false
	}
	return w.Name == name || w.ID == name
}

func (f *Fake) window(target string) *fakeWindow {
	sess, name, ok := strings.Cut(target, ":")
	if !ok {
		return nil
	}
	s, ok := f.Sessions[sess]
	if !ok {
		return nil
	}
	for _, w := range s.Windows {
		if windowMatches(w, name) {
			return w
		}
	}
	return nil
}

func nextPID(session, window string) int {
	h := 1000
	for _, c := range session + window {
		h = h*33 + int(c)
	}
	if h < 0 {
		h = -h
	}
	return 10000 + h%50000
}
