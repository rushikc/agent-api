package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/rushikesh/devhub/internal/client"
	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/daemon"
	"github.com/rushikesh/devhub/internal/registry"
)

func RunE2E() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	_ = daemon.Ensure(cfg)
	c := client.Unix(cfg, os.Getenv("DEVHUB_CALLER"))
	m := e2eModel{c: c}
	m.refresh()
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

type e2eModel struct {
	c     *client.Client
	runs  []registry.E2ERun
	cur   int
	logs  bool
	log   string
	vp    viewport.Model
	err   string
	width int
}

type e2eTick struct{}

func (m e2eModel) Init() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return e2eTick{} })
}

func (m *e2eModel) refresh() {
	b, err := m.c.E2EList("")
	if err != nil {
		m.err = err.Error()
		return
	}
	_ = json.Unmarshal(b, &m.runs)
}

func (m e2eModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.vp = viewport.New(msg.Width, max(8, msg.Height-6))
	case e2eTick:
		m.refresh()
		if m.logs && m.cur < len(m.runs) {
			m.log, _ = m.c.E2ELogs(m.runs[m.cur].ID)
			m.vp.SetContent(m.log)
		}
		return m, tea.Tick(time.Second, func(time.Time) tea.Msg { return e2eTick{} })
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			if m.logs {
				m.logs = false
				return m, nil
			}
			return m, tea.Quit
		case "up", "k":
			if m.cur > 0 {
				m.cur--
			}
		case "down", "j":
			if m.cur < len(m.runs)-1 {
				m.cur++
			}
		case "enter":
			if m.cur < len(m.runs) {
				m.logs = true
				m.log, _ = m.c.E2ELogs(m.runs[m.cur].ID)
				m.vp.SetContent(m.log)
			}
		case "r":
			if m.cur < len(m.runs) {
				_, err := m.c.E2ERun(m.runs[m.cur].Project, map[string]any{})
				if err != nil {
					m.err = err.Error()
				}
			}
		case "o":
			if m.cur < len(m.runs) {
				p := m.runs[m.cur].ArtifactPath
				html := filepath.Join(p, "index.html")
				_ = exec.Command("open", html).Start()
			}
		}
	}
	if m.logs {
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m e2eModel) View() string {
	if m.logs {
		return renderHeader("E2E logs", "", m.width) + "\n" + m.vp.View()
	}
	s := renderHeader("E2E", "enter logs · r rerun · o report", m.width) + "\n"
	for i, r := range m.runs {
		line := fmt.Sprintf("%s  %s  %s  %s", r.ID, r.Project, r.Status, r.GitSHA)
		if i == m.cur {
			line = selStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		s += line + "\n"
	}
	s += renderError(m.err)
	return s
}
