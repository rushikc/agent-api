package tmux

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type CLI struct {
	Socket string
	Path   string // tmux binary, empty = look up
}

func New() *CLI {
	return &CLI{Socket: SocketName}
}

func (c *CLI) bin() string {
	if c.Path != "" {
		return c.Path
	}
	return "tmux"
}

func (c *CLI) args(rest ...string) []string {
	sock := c.Socket
	if sock == "" {
		sock = SocketName
	}
	return append([]string{"-L", sock}, rest...)
}

func (c *CLI) cmd(rest ...string) *exec.Cmd {
	return exec.Command(c.bin(), c.args(rest...)...)
}

func (c *CLI) output(rest ...string) (string, error) {
	cmd := c.cmd(rest...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.Error); ok && ee.Err == exec.ErrNotFound {
			return "", MissingTmuxError()
		}
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", fmt.Errorf("tmux %s: %s", strings.Join(rest, " "), msg)
		}
		return "", fmt.Errorf("tmux %s: %w", strings.Join(rest, " "), err)
	}
	return string(out), nil
}

func (c *CLI) run(rest ...string) error {
	_, err := c.output(rest...)
	return err
}

func (c *CLI) Version() (string, error) {
	cmd := exec.Command(c.bin(), "-V")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.Error); ok && ee.Err == exec.ErrNotFound {
			return "", MissingTmuxError()
		}
		return "", fmt.Errorf("tmux -V: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *CLI) CheckVersion() error {
	v, err := c.Version()
	if err != nil {
		return err
	}
	maj, min, err := ParseVersion(v)
	if err != nil {
		return err
	}
	if !VersionOK(maj, min) {
		return OldTmuxError(v)
	}
	return nil
}

func (c *CLI) EnsureServer() error {
	if err := c.CheckVersion(); err != nil {
		return err
	}
	return c.run("start-server")
}

func (c *CLI) HasSession(name string) (bool, error) {
	cmd := c.cmd("has-session", "-t", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if ee, ok := err.(*exec.Error); ok && ee.Err == exec.ErrNotFound {
		return false, MissingTmuxError()
	}
	if _, ok := err.(*exec.ExitError); ok {
		return false, nil
	}
	return false, err
}

func requireCwd(cwd string) error {
	if strings.TrimSpace(cwd) == "" {
		return fmt.Errorf("cwd is required (-c); refusing to start in $HOME")
	}
	return nil
}

func envArgs(env map[string]string) []string {
	var args []string
	for k, v := range env {
		if k == "" {
			continue
		}
		args = append(args, "-e", k+"="+v)
	}
	return args
}

func (c *CLI) NewSession(opts SessionOpts) error {
	if err := requireCwd(opts.Cwd); err != nil {
		return err
	}
	args := []string{"new-session", "-d", "-s", opts.Name, "-c", opts.Cwd}
	if opts.Window != "" {
		args = append(args, "-n", opts.Window)
	}
	args = append(args, envArgs(opts.Env)...)
	args = append(args, opts.Command...)
	if err := c.run(args...); err != nil {
		return err
	}
	if opts.RemainOnExit {
		target := opts.Name
		if opts.Window != "" {
			target = opts.Name + ":" + opts.Window
		}
		return c.SetRemainOnExit(target, true)
	}
	return nil
}

func (c *CLI) NewWindow(opts WindowOpts) error {
	if err := requireCwd(opts.Cwd); err != nil {
		return err
	}
	args := []string{"new-window", "-t", opts.Session, "-n", opts.Name, "-c", opts.Cwd}
	args = append(args, envArgs(opts.Env)...)
	args = append(args, opts.Command...)
	if err := c.run(args...); err != nil {
		return err
	}
	if opts.RemainOnExit {
		return c.SetRemainOnExit(opts.Session+":"+opts.Name, true)
	}
	return nil
}

func (c *CLI) SetEnvironment(target, key, value string) error {
	if target == "" {
		return c.run("set-environment", key, value)
	}
	return c.run("set-environment", "-t", target, key, value)
}

func (c *CLI) SetDefaultPath(session, path string) error {
	if err := requireCwd(path); err != nil {
		return err
	}
	return c.run("set-option", "-t", session, "default-path", path)
}

func (c *CLI) SetUserOption(target, key, value string) error {
	if !strings.HasPrefix(key, "@") {
		key = "@" + key
	}
	return c.run("set-option", "-t", target, key, value)
}

func (c *CLI) RespawnPane(opts RespawnOpts) error {
	if err := requireCwd(opts.Cwd); err != nil {
		return err
	}
	args := []string{"respawn-pane", "-k", "-c", opts.Cwd, "-t", opts.Target}
	args = append(args, envArgs(opts.Env)...)
	args = append(args, opts.Command...)
	return c.run(args...)
}

func (c *CLI) SendKeys(target, keys string) error {
	return c.run("send-keys", "-t", target, keys)
}

func (c *CLI) HasPane(target string) (bool, error) {
	panes, err := c.ListPanes()
	if err != nil {
		return false, err
	}
	for _, p := range panes {
		if p.Target == target {
			return true, nil
		}
	}
	return false, nil
}

func (c *CLI) SetRemainOnExit(target string, on bool) error {
	val := "off"
	if on {
		val = "on"
	}
	return c.run("set-option", "-t", target, "remain-on-exit", val)
}

func (c *CLI) ListPanes() ([]Pane, error) {
	format := strings.Join([]string{
		"#{session_name}",
		"#{window_name}",
		"#{window_id}",
		"#{pane_id}",
		"#{pane_pid}",
		"#{pane_dead}",
		"#{pane_dead_status}",
		"#{pane_current_path}",
		"#{session_name}:#{window_name}",
	}, "\t")
	out, err := c.output("list-panes", "-a", "-F", format)
	if err != nil {
		if strings.Contains(err.Error(), "no server running") || strings.Contains(err.Error(), "no sessions") {
			return nil, nil
		}
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	var panes []Pane
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		p, err := parsePane(line)
		if err != nil {
			continue
		}
		if !IsDevhubSession(p.Session) {
			continue
		}
		panes = append(panes, p)
	}
	return panes, nil
}

func parsePane(line string) (Pane, error) {
	parts := strings.Split(line, "\t")
	if len(parts) < 9 {
		return Pane{}, fmt.Errorf("bad pane line")
	}
	pid, _ := strconv.Atoi(parts[4])
	dead := parts[5] == "1"
	status := -1
	if parts[6] != "" {
		if n, err := strconv.Atoi(parts[6]); err == nil {
			status = n
		}
	}
	return Pane{
		Session:    parts[0],
		Window:     parts[1],
		WindowID:   parts[2],
		PaneID:     parts[3],
		PID:        pid,
		Dead:       dead,
		DeadStatus: status,
		Path:       parts[7],
		Target:     parts[8],
	}, nil
}

func (c *CLI) ListSessions() ([]Session, error) {
	out, err := c.output("list-sessions", "-F", "#{session_name}\t#{session_attached}")
	if err != nil {
		if strings.Contains(err.Error(), "no server running") || strings.Contains(err.Error(), "no sessions") {
			return nil, nil
		}
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	var ss []Session
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		s := Session{Name: parts[0]}
		if len(parts) > 1 && parts[1] != "0" && parts[1] != "" {
			s.Attached = true
		}
		ss = append(ss, s)
	}
	return ss, nil
}

func (c *CLI) CapturePane(target string, start int) (string, error) {
	out, err := c.output("capture-pane", "-p", "-t", target, "-S", strconv.Itoa(start))
	if err != nil {
		return "", err
	}
	return out, nil
}

func (c *CLI) SwitchClient(target string) error {
	if os.Getenv("TMUX") != "" {
		return c.run("switch-client", "-t", target)
	}
	cmd := c.cmd("attach-session", "-t", target)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *CLI) KillWindow(target string) error {
	return c.run("kill-window", "-t", target)
}

func (c *CLI) RenameWindow(target, name string) error {
	return c.run("rename-window", "-t", target, name)
}

func (c *CLI) MoveWindow(src, dstSession string) error {
	dst := dstSession
	if !strings.HasSuffix(dst, ":") {
		dst += ":"
	}
	return c.run("move-window", "-s", src, "-t", dst)
}

func (c *CLI) SourceFile(path string) error {
	return c.run("source-file", path)
}

func (c *CLI) CurrentSession() (string, error) {
	out, err := c.output("display-message", "-p", "#{session_name}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
