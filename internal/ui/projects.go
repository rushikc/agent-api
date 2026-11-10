package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rushikesh/devhub/internal/core"
)

func RunProjects(svc *core.Service) error {
	m := newProjects(svc)
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

type projectsModel struct {
	svc     *core.Service
	names   []string
	paths   map[string]string
	cursor  int
	filter  string
	err     string
	width   int
	height  int
}

func newProjects(svc *core.Service) projectsModel {
	names := make([]string, 0, len(svc.Cfg.Projects))
	for n := range svc.Cfg.Projects {
		names = append(names, n)
	}
	sort.Strings(names)
	return projectsModel{svc: svc, names: names, paths: svc.Cfg.Projects}
}

func (m projectsModel) filtered() []string {
	var out []string
	for _, n := range m.names {
		if filterMatch(m.filter, n+" "+m.paths[n]) {
			out = append(out, n)
		}
	}
	return out
}

func (m projectsModel) Init() tea.Cmd { return nil }

func (m projectsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		list := m.filtered()
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(list)-1 {
				m.cursor++
			}
		case "enter":
			if m.cursor < 0 || m.cursor >= len(list) {
				return m, nil
			}
			if err := m.svc.SwitchProject(list[m.cursor]); err != nil {
				m.err = err.Error()
				return m, nil
			}
			return m, tea.Quit
		case "backspace":
			if m.filter != "" {
				m.filter = m.filter[:len(m.filter)-1]
				m.cursor = 0
			}
		default:
			if len(msg.Runes) == 1 && msg.Type == tea.KeyRunes {
				m.filter += string(msg.Runes)
				m.cursor = 0
			}
		}
	}
	return m, nil
}

func (m projectsModel) View() string {
	var b strings.Builder
	b.WriteString(renderHeader("projects", "enter to switch", m.width) + "\n")
	if m.filter != "" {
		b.WriteString(mutedStyle.Render("filter: "+m.filter) + "\n")
	}
	list := m.filtered()
	if len(list) == 0 {
		b.WriteString(mutedStyle.Render("  no projects (devhub project add <name> <path>)") + "\n")
	}
	for i, n := range list {
		cur := "  "
		st := lipgloss.NewStyle()
		if i == m.cursor {
			cur = "▸ "
			st = selStyle
		}
		b.WriteString(st.Render(fmt.Sprintf("%s%-16s  %s", cur, n, m.paths[n])) + "\n")
	}
	b.WriteString("\n" + renderError(m.err) + "\n")
	b.WriteString(renderHelp([]string{"enter switch", "type to filter", "esc/q close"}, m.width))
	return b.String()
}
