package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/registry"
)

func RunPicker(svc *core.Service) error {
	m, err := newPicker(svc)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

type pickerMode int

const (
	pickerList pickerMode = iota
	pickerConfirm
	pickerRename
	pickerNewProject
)

type pickerModel struct {
	svc      *core.Service
	items    []core.SessionView
	filtered []core.SessionView
	cursor   int
	filter   string
	mode     pickerMode
	input    textinput.Model
	confirm  string
	err      string
	width    int
	height   int
	projects []string
	pCursor  int
}

func newPicker(svc *core.Service) (pickerModel, error) {
	items, err := svc.List(false)
	if err != nil {
		return pickerModel{}, err
	}
	var active []core.SessionView
	for _, it := range items {
		if it.Session.Kind == registry.KindActive {
			active = append(active, it)
		}
	}
	ti := textinput.New()
	ti.Placeholder = "new name"
	ti.CharLimit = 40
	var projects []string
	for n := range svc.Cfg.Projects {
		projects = append(projects, n)
	}
	m := pickerModel{svc: svc, items: active, input: ti, projects: projects}
	m.applyFilter()
	return m, nil
}

func (m *pickerModel) applyFilter() {
	m.filtered = nil
	for _, it := range m.items {
		if filterMatch(m.filter, sessionHaystack(it)) {
			m.filtered = append(m.filtered, it)
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m pickerModel) current() (core.SessionView, bool) {
	if len(m.filtered) == 0 || m.cursor < 0 || m.cursor >= len(m.filtered) {
		return core.SessionView{}, false
	}
	return m.filtered[m.cursor], true
}

func (m pickerModel) Init() tea.Cmd { return nil }

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		if m.mode == pickerRename {
			return m.updateRename(msg)
		}
		if m.mode == pickerConfirm {
			return m.updateConfirm(msg)
		}
		if m.mode == pickerNewProject {
			return m.updateNewProject(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m pickerModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case "enter":
		it, ok := m.current()
		if !ok {
			return m, nil
		}
		if err := m.svc.Attach(it.Session.Project + "/" + it.Session.Name); err != nil {
			m.err = err.Error()
			return m, nil
		}
		return m, tea.Quit
	case "n":
		if len(m.projects) == 0 {
			m.err = "no projects (devhub project add <name> <path>)"
			return m, nil
		}
		if len(m.projects) == 1 {
			if _, err := m.svc.NewActive(m.projects[0], ""); err != nil {
				m.err = err.Error()
				return m, nil
			}
			return m.reload()
		}
		m.mode = pickerNewProject
		m.pCursor = 0
	case "r":
		it, ok := m.current()
		if !ok {
			return m, nil
		}
		m.mode = pickerRename
		m.input.SetValue(it.Session.Name)
		m.input.Focus()
		return m, textinput.Blink
	case "x":
		it, ok := m.current()
		if !ok {
			return m, nil
		}
		m.mode = pickerConfirm
		m.confirm = it.Session.Project + "/" + it.Session.Name
	case "backspace":
		if m.filter != "" {
			m.filter = m.filter[:len(m.filter)-1]
			m.applyFilter()
		}
	default:
		if len(msg.Runes) == 1 && msg.Type == tea.KeyRunes {
			m.filter += string(msg.Runes)
			m.applyFilter()
		}
	}
	return m, nil
}

func (m pickerModel) updateRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = pickerList
		m.input.Blur()
		return m, nil
	case "enter":
		it, ok := m.current()
		if !ok {
			m.mode = pickerList
			return m, nil
		}
		name := strings.TrimSpace(m.input.Value())
		if err := m.svc.Rename(it.Session.Project+"/"+it.Session.Name, name); err != nil {
			m.err = err.Error()
		}
		m.mode = pickerList
		m.input.Blur()
		return m.reload()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m pickerModel) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		if err := m.svc.Kill(m.confirm); err != nil {
			m.err = err.Error()
		}
		m.mode = pickerList
		return m.reload()
	case "n", "esc", "q":
		m.mode = pickerList
	}
	return m, nil
}

func (m pickerModel) updateNewProject(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.mode = pickerList
	case "up", "k":
		if m.pCursor > 0 {
			m.pCursor--
		}
	case "down", "j":
		if m.pCursor < len(m.projects)-1 {
			m.pCursor++
		}
	case "enter":
		if _, err := m.svc.NewActive(m.projects[m.pCursor], ""); err != nil {
			m.err = err.Error()
		}
		m.mode = pickerList
		return m.reload()
	}
	return m, nil
}

func (m pickerModel) reload() (tea.Model, tea.Cmd) {
	items, err := m.svc.List(false)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.items = nil
	for _, it := range items {
		if it.Session.Kind == registry.KindActive {
			m.items = append(m.items, it)
		}
	}
	m.applyFilter()
	return m, nil
}

func (m pickerModel) View() string {
	var b strings.Builder
	b.WriteString(renderHeader("devhub picker", "active terminals", m.width) + "\n")
	if m.filter != "" {
		b.WriteString(mutedStyle.Render("filter: "+m.filter) + "\n")
	}
	if m.mode == pickerConfirm {
		b.WriteString(errStyle.Render(fmt.Sprintf("kill %s? [y/n]", m.confirm)) + "\n")
	}
	if m.mode == pickerRename {
		b.WriteString("rename: " + m.input.View() + "\n")
	}
	if m.mode == pickerNewProject {
		b.WriteString(mutedStyle.Render("new terminal in project:") + "\n")
		for i, p := range m.projects {
			cur := "  "
			st := lipgloss.NewStyle()
			if i == m.pCursor {
				cur = "▸ "
				st = selStyle
			}
			b.WriteString(st.Render(cur+p) + "\n")
		}
	} else {
		bodyH := m.height - 5
		if bodyH < 3 {
			bodyH = 8
		}
		if len(m.filtered) == 0 {
			b.WriteString(mutedStyle.Render("  no active terminals") + "\n")
		}
		for i, it := range m.filtered {
			if i >= bodyH {
				break
			}
			s := it.Session
			cur := "  "
			st := rowStyle(s.Kind, i == m.cursor)
			if i == m.cursor {
				cur = "▸ "
			}
			stat := statusStyle(s.Status).Render(string(s.Status))
			line := fmt.Sprintf("%s%-12s  %-16s  %s", cur, s.Project, s.Name, stat)
			b.WriteString(st.Render(line) + "\n")
		}
	}
	b.WriteString("\n" + renderError(m.err) + "\n")
	b.WriteString(renderHelp([]string{"enter switch", "n new", "r rename", "x kill", "type to filter", "esc/q close"}, m.width))
	return b.String()
}
