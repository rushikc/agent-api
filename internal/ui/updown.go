package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/profile"
)

func RunUpDown(svc *core.Service) error {
	m := updownModel{svc: svc}
	m.refresh()
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

type updownModel struct {
	svc      *core.Service
	profiles []profile.Profile
	cursor   int
	err      string
	width    int
	log      string
}

func (m *updownModel) refresh() {
	m.profiles = profile.List(m.svc.Cfg)
}

func (m updownModel) Init() tea.Cmd { return nil }

func (m updownModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.profiles)-1 {
				m.cursor++
			}
		case "u", "enter":
			if m.cursor < len(m.profiles) {
				p := m.profiles[m.cursor]
				pr, err := profile.Load(p.Path)
				if err != nil {
					m.err = err.Error()
					return m, nil
				}
				rows, err := profile.Up(m.svc, pr, true)
				if err != nil {
					m.err = err.Error()
				} else {
					m.log = fmt.Sprintf("up %d shadows", len(rows))
					m.err = ""
				}
			}
		case "d":
			if m.cursor < len(m.profiles) {
				p := m.profiles[m.cursor]
				if err := profile.Down(m.svc, p.Name, false); err != nil {
					m.err = err.Error()
				} else {
					m.log = "down " + p.Name
				}
			}
		}
	}
	return m, nil
}

func (m updownModel) View() string {
	var b strings.Builder
	b.WriteString(renderHeader("profiles", "up / down", m.width) + "\n")
	if len(m.profiles) == 0 {
		b.WriteString(mutedStyle.Render("  no registered projects with a path") + "\n")
	}
	for i, p := range m.profiles {
		cur := "  "
		st := lipgloss.NewStyle()
		if i == m.cursor {
			cur = "▸ "
			st = selStyle
		}
		b.WriteString(st.Render(fmt.Sprintf("%s%-16s  %s", cur, p.Name, p.Path)) + "\n")
	}
	if m.log != "" {
		b.WriteString("\n" + mutedStyle.Render(m.log) + "\n")
	}
	b.WriteString("\n" + renderError(m.err) + "\n")
	b.WriteString(renderHelp([]string{"enter/u up", "d down", "esc/q"}, m.width))
	return b.String()
}
