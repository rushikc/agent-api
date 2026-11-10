package tmux

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrNoServer = errors.New("tmux: no server running")

const (
	SocketName    = "devhub"
	MinMajor      = 3
	MinMinor      = 3
	MainSession   = "dh-main"
	SessionPrefix = "dh-"
	ShadowPrefix  = "dh-shadow-"
	MainWindow    = "main"
)

type SessionOpts struct {
	Name         string
	Window       string
	Cwd          string
	Command      []string
	Env          map[string]string
	RemainOnExit bool
}

type WindowOpts struct {
	Session      string
	Name         string
	Cwd          string
	Command      []string
	Env          map[string]string
	RemainOnExit bool
}

type RespawnOpts struct {
	Target  string
	Cwd     string
	Command []string
	Env     map[string]string
}

type Pane struct {
	Session    string
	Window     string
	WindowID   string
	PaneID     string
	PID        int
	Dead       bool
	DeadStatus int // -1 if unknown
	Path       string
	Target     string
}

type Session struct {
	Name     string
	Attached bool
}

// Client is the tmux CLI surface. Tests should use Fake.
type Client interface {
	Version() (string, error)
	EnsureServer() error
	HasSession(name string) (bool, error)
	NewSession(opts SessionOpts) error
	NewWindow(opts WindowOpts) error
	ListPanes() ([]Pane, error)
	ListSessions() ([]Session, error)
	CapturePane(target string, start int) (string, error)
	SwitchClient(target string) error
	KillWindow(target string) error
	RenameWindow(target, name string) error
	MoveWindow(src, dstSession string) error
	SourceFile(path string) error
	CurrentSession() (string, error)
	SetRemainOnExit(target string, on bool) error
	SetEnvironment(target, key, value string) error
	SetUserOption(target, key, value string) error
	RespawnPane(opts RespawnOpts) error
	HasPane(target string) (bool, error)
	SendKeys(target, keys string) error
}

func ProjectSession(project string) string {
	return SessionPrefix + project
}

func ShadowSession(project string) string {
	return ShadowPrefix + project
}

func ProjectFromSession(name string) (string, bool) {
	if name == "" || name == MainSession {
		return "", false
	}
	if strings.HasPrefix(name, ShadowPrefix) {
		return "", false
	}
	if strings.HasPrefix(name, SessionPrefix) {
		return strings.TrimPrefix(name, SessionPrefix), true
	}
	return "", false
}

func IsDevhubSession(name string) bool {
	return strings.HasPrefix(name, SessionPrefix)
}

func IsShadowSession(name string) bool {
	return strings.HasPrefix(name, ShadowPrefix)
}

func ParseVersion(s string) (major, minor int, err error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "tmux ")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, fmt.Errorf("empty tmux version")
	}
	// 3.3a, 3.4, 3.5a, next-3.4
	s = strings.TrimPrefix(s, "next-")
	var maj, min int
	n, err := fmt.Sscanf(s, "%d.%d", &maj, &min)
	if n < 2 || err != nil {
		// try stripping trailing letters from minor by scanning digits
		parts := strings.SplitN(s, ".", 2)
		if len(parts) != 2 {
			return 0, 0, fmt.Errorf("unrecognized tmux version %q", s)
		}
		maj, err = strconv.Atoi(parts[0])
		if err != nil {
			return 0, 0, fmt.Errorf("unrecognized tmux version %q", s)
		}
		minStr := parts[1]
		i := 0
		for i < len(minStr) && minStr[i] >= '0' && minStr[i] <= '9' {
			i++
		}
		if i == 0 {
			return 0, 0, fmt.Errorf("unrecognized tmux version %q", s)
		}
		min, _ = strconv.Atoi(minStr[:i])
	}
	return maj, min, nil
}

func VersionOK(major, minor int) bool {
	if major > MinMajor {
		return true
	}
	if major < MinMajor {
		return false
	}
	return minor >= MinMinor
}

func MissingTmuxError() error {
	return fmt.Errorf("tmux is not installed or not on PATH\nInstall it with: brew install tmux")
}

func OldTmuxError(version string) error {
	return fmt.Errorf("tmux %s is too old (need >= 3.3 for display-popup)\nUpgrade with: brew upgrade tmux", version)
}
