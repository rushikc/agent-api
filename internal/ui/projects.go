package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/scan"
)

func RunProjects(svc *core.Service) error {
	m := newProjects(svc)
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

type projectsMode int

const (
	projectsList projectsMode = iota
	projectsConfirm
	projectsKill
	projectsRename
)

type projectsModel struct {
	svc    *core.Service
	names  []string
	paths  map[string]string
	cursor int
	filter string
	err    string
	width  int
	height int
	mode   projectsMode
	input  textinput.Model
	target string
}

func newProjects(svc *core.Service) projectsModel {
	ti := textinput.New()
	ti.CharLimit = 48
	m := projectsModel{svc: svc, input: ti}
	m.reload()
	return m
}

func (m *projectsModel) reload() {
	names := make([]string, 0, len(m.svc.Cfg.Projects))
	for n := range m.svc.Cfg.Projects {
		names = append(names, n)
	}
	sort.Strings(names)
	m.names = names
	m.paths = m.svc.Cfg.Projects
	if m.cursor >= len(names) {
		m.cursor = max(0, len(names)-1)
	}
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
		if m.mode == projectsRename {
			switch msg.String() {
			case "esc":
				m.mode = projectsList
				return m, nil
			case "enter":
				newName := strings.TrimSpace(m.input.Value())
				if newName != "" && m.target != "" {
					if err := m.svc.RenameProject(m.target, newName); err != nil {
						m.err = err.Error()
					} else {
						m.reload()
					}
				}
				m.mode = projectsList
				return m, nil
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		if m.mode == projectsConfirm {
			switch msg.String() {
			case "esc", "n", "q":
				m.mode = projectsList
			case "enter", "y":
				if m.svc.ProjectHasSessions(m.target) {
					m.mode = projectsKill
					return m, nil
				}
				if err := m.svc.RemoveProject(m.target, false); err != nil {
					m.err = err.Error()
				}
				m.reload()
				m.mode = projectsList
			}
			return m, nil
		}
		if m.mode == projectsKill {
			switch msg.String() {
			case "esc", "n", "q":
				m.mode = projectsList
			case "enter":
				if err := m.svc.RemoveProject(m.target, false); err != nil {
					m.err = err.Error()
				}
				m.reload()
				m.mode = projectsList
			case "k":
				if err := m.svc.RemoveProject(m.target, true); err != nil {
					m.err = err.Error()
				}
				m.reload()
				m.mode = projectsList
			}
			return m, nil
		}
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
		case "x":
			if m.cursor >= 0 && m.cursor < len(list) {
				m.target = list[m.cursor]
				m.mode = projectsConfirm
			}
		case "e":
			if m.cursor >= 0 && m.cursor < len(list) {
				m.target = list[m.cursor]
				m.input.SetValue(m.target)
				m.input.Focus()
				m.mode = projectsRename
				return m, textinput.Blink
			}
		case "+":
			sm := openScanFromCache(m.svc)
			sm.parent = m
			sm.width, sm.height = m.width, m.height
			return sm, nil
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

func openScanFromCache(svc *core.Service) scanModel {
	var res *scan.Result
	var age time.Duration
	fromCache := false
	if svc != nil && svc.Cfg != nil {
		if c, err := scan.LoadCache(svc.Cfg.ScanCachePath()); err == nil && c != nil {
			res = scan.ResultFromCache(c)
			scan.Annotate(res, svc.Cfg.Projects)
			age = scan.Age(c)
			fromCache = true
		}
	}
	if res == nil {
		res = &scan.Result{}
	}
	return newScanModel(svc, res, age, fromCache)
}

func (m projectsModel) View() string {
	var b strings.Builder
	b.WriteString(renderHeader("projects", "enter to switch", m.width) + "\n")
	if m.mode == projectsConfirm {
		b.WriteString(errStyle.Render("remove "+m.target+"? [enter/y] [esc]") + "\n")
		return b.String()
	}
	if m.mode == projectsKill {
		b.WriteString(errStyle.Render(m.target+" has sessions. enter=keep  k=kill  esc=cancel") + "\n")
		return b.String()
	}
	if m.mode == projectsRename {
		b.WriteString("rename: " + m.input.View() + "\n")
		return b.String()
	}
	if m.filter != "" {
		b.WriteString(mutedStyle.Render("filter: "+m.filter) + "\n")
	}
	list := m.filtered()
	if len(list) == 0 {
		b.WriteString(mutedStyle.Render("  no projects (+ to scan)") + "\n")
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
	b.WriteString(renderHelp([]string{"enter switch", "x remove", "e rename", "+ scan", "type to filter", "esc/q close"}, m.width))
	return b.String()
}
