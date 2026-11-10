package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/janitor"
)

func RunJanitor(svc *core.Service) error {
	m := janitorModel{svc: svc, tab: 0}
	m.refresh()
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

type janitorModel struct {
	svc     *core.Service
	tab     int
	ports   []janitor.Port
	orphans []janitor.Orphan
	cursor  int
	confirm bool
	detail  string
	err     string
	width   int
}

func (m *janitorModel) refresh() {
	rows, _ := m.svc.DB.List(false)
	m.ports, _ = janitor.ListPorts(rows, nil)
	m.orphans, _ = janitor.ListOrphans(m.svc.Cfg, rows, nil)
}

func (m janitorModel) Init() tea.Cmd { return nil }

func (m janitorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		if m.confirm {
			switch msg.String() {
			case "y", "enter":
				if m.tab == 0 && m.cursor < len(m.ports) {
					_ = janitor.KillTree(m.ports[m.cursor].PID)
				}
				if m.tab == 1 && m.cursor < len(m.orphans) {
					_ = janitor.KillTree(m.orphans[m.cursor].PID)
				}
				m.confirm = false
				m.refresh()
			default:
				m.confirm = false
			}
			return m, nil
		}
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "right", "l":
			m.tab = 1 - m.tab
			m.cursor = 0
		case "left", "h":
			m.tab = 1 - m.tab
			m.cursor = 0
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			n := len(m.ports)
			if m.tab == 1 {
				n = len(m.orphans)
			}
			if m.cursor < n-1 {
				m.cursor++
			}
		case "x":
			m.confirm = true
		case "a":
			if m.tab == 1 && m.cursor < len(m.orphans) {
				o := m.orphans[m.cursor]
				_, err := janitor.Adopt(m.svc.DB, o.Project, "", o.PID, o.Cwd, o.Name)
				if err != nil {
					m.err = err.Error()
				}
				m.refresh()
			}
		case "enter":
			if m.tab == 0 && m.cursor < len(m.ports) {
				p := m.ports[m.cursor]
				m.detail = fmt.Sprintf(":%d pid=%d owner=%s cmd=%s", p.Port, p.PID, p.Owner, p.Cmd)
			}
			if m.tab == 1 && m.cursor < len(m.orphans) {
				o := m.orphans[m.cursor]
				m.detail = fmt.Sprintf("pid=%d %s cwd=%s project=%s", o.PID, o.Name, o.Cwd, o.Project)
			}
		}
	}
	return m, nil
}

func (m janitorModel) View() string {
	var b strings.Builder
	tab := "ports"
	if m.tab == 1 {
		tab = "orphans"
	}
	b.WriteString(renderHeader("janitor", tab, m.width) + "\n")
	if m.confirm {
		b.WriteString(errStyle.Render("kill selected? [y/n]") + "\n")
	}
	if m.tab == 0 {
		for i, p := range m.ports {
			cur := "  "
			st := lipgloss.NewStyle()
			if i == m.cursor {
				cur = "▸ "
				st = selStyle
			}
			b.WriteString(st.Render(fmt.Sprintf("%s:%-5d  pid %-6d  %-16s  %s", cur, p.Port, p.PID, p.Owner, p.Cmd)) + "\n")
		}
	} else {
		for i, o := range m.orphans {
			cur := "  "
			st := lipgloss.NewStyle()
			if i == m.cursor {
				cur = "▸ "
				st = selStyle
			}
			b.WriteString(st.Render(fmt.Sprintf("%s%-8s pid %-6d  %s  %s", cur, o.Name, o.PID, o.Project, o.Cwd)) + "\n")
		}
	}
	if m.detail != "" {
		b.WriteString("\n" + mutedStyle.Render(m.detail) + "\n")
	}
	b.WriteString("\n" + renderError(m.err) + "\n")
	b.WriteString(renderHelp([]string{"tab ports/orphans", "x kill", "a adopt", "enter details", "esc/q"}, m.width))
	return b.String()
}
