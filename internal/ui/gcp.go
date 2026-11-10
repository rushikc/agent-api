package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rushikesh/devhub/internal/client"
	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/daemon"
)

func RunGCP() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	_ = daemon.Ensure(cfg)
	c := client.Unix(cfg, os.Getenv("DEVHUB_CALLER"))
	m := gcpModel{c: c, step: 0, input: textinput.New()}
	m.input.Placeholder = "value"
	m.input.Focus()
	b, err := c.GCPProjects()
	if err != nil {
		return err
	}
	_ = json.Unmarshal(b, &m.projects)
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

type gcpProj struct {
	ID             string   `json:"id"`
	Alias          string   `json:"alias"`
	Allow          []string `json:"allow"`
	RequireConfirm bool     `json:"require_confirm"`
}

type gcpModel struct {
	c        *client.Client
	projects []gcpProj
	tasks    []string
	step     int
	pcur     int
	tcur     int
	input    textinput.Model
	vp       viewport.Model
	result   string
	err      string
	width    int
}

func (m gcpModel) Init() tea.Cmd { return nil }

func (m gcpModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.vp = viewport.New(msg.Width, max(5, msg.Height-6))
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			if m.step > 0 && m.step < 3 {
				m.step--
				return m, nil
			}
			return m, tea.Quit
		case "up", "k":
			if m.step == 0 && m.pcur > 0 {
				m.pcur--
			}
			if m.step == 1 && m.tcur > 0 {
				m.tcur--
			}
		case "down", "j":
			if m.step == 0 && m.pcur < len(m.projects)-1 {
				m.pcur++
			}
			if m.step == 1 && m.tcur < len(m.tasks)-1 {
				m.tcur++
			}
		case "enter":
			if m.step == 0 && m.pcur < len(m.projects) {
				m.tasks = m.projects[m.pcur].Allow
				m.tcur = 0
				m.step = 1
			} else if m.step == 1 && m.tcur < len(m.tasks) {
				m.step = 2
				m.input.SetValue("")
			} else if m.step == 2 {
				p := m.projects[m.pcur]
				task := m.tasks[m.tcur]
				params := map[string]any{}
				switch task {
				case "logs.read":
					params["filter"] = m.input.Value()
					params["since"] = "1h"
				case "bq.query":
					params["sql"] = m.input.Value()
				case "pubsub.peek":
					params["subscription"] = m.input.Value()
				case "gcs.list":
					params["bucket"] = m.input.Value()
				}
				b, err := m.c.GCPCall(p.Alias, task, params, true, false)
				if err != nil {
					m.err = err.Error()
				} else {
					m.err = ""
					m.result = colorizeGCP(task, string(b))
					m.vp.SetContent(m.result)
					m.step = 3
				}
			}
		}
	}
	if m.step == 2 {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	if m.step == 3 {
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m gcpModel) View() string {
	switch m.step {
	case 0:
		var b strings.Builder
		b.WriteString(renderHeader("GCP", "pick project", m.width) + "\n")
		for i, p := range m.projects {
			line := fmt.Sprintf("%s  %s  %s", p.Alias, p.ID, strings.Join(p.Allow, ","))
			if i == m.pcur {
				line = selStyle.Render("> " + line)
			} else {
				line = "  " + line
			}
			b.WriteString(line + "\n")
		}
		b.WriteString(renderHelp([]string{"enter pick", "esc quit"}, m.width))
		return b.String()
	case 1:
		var b strings.Builder
		b.WriteString(renderHeader("GCP "+m.projects[m.pcur].Alias, "pick task", m.width) + "\n")
		for i, t := range m.tasks {
			line := t
			if i == m.tcur {
				line = selStyle.Render("> " + line)
			} else {
				line = "  " + line
			}
			b.WriteString(line + "\n")
		}
		return b.String()
	case 2:
		return renderHeader("GCP", m.tasks[m.tcur], m.width) + "\n" + m.input.View() + "\n" + renderError(m.err)
	default:
		return renderHeader("GCP result", "", m.width) + "\n" + m.vp.View()
	}
}

func colorizeGCP(task, raw string) string {
	if task == "logs.read" {
		raw = strings.ReplaceAll(raw, `"ERROR"`, exitStyle.Render(`"ERROR"`))
		raw = strings.ReplaceAll(raw, `"WARNING"`, lipgloss.NewStyle().Foreground(colorAccent).Render(`"WARNING"`))
	}
	return raw
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
