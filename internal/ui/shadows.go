package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/registry"
)

func RunShadows(svc *core.Service) error {
	m, err := newShadows(svc)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

type shadowMode int

const (
	shadowList shadowMode = iota
	shadowPeek
	shadowConfirm
	shadowPickProject
	shadowCommand
)

type tickMsg time.Time

func peekTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

type shadowsModel struct {
	svc      *core.Service
	items    []core.SessionView
	filtered []core.SessionView
	cursor   int
	filter   string
	mode     shadowMode
	vp       viewport.Model
	peekRef  string
	input    textinput.Model
	projects []string
	pCursor  int
	confirm  string
	err      string
	width    int
	height   int
}

func newShadows(svc *core.Service) (shadowsModel, error) {
	items, err := svc.List(true)
	if err != nil {
		return shadowsModel{}, err
	}
	var shadows []core.SessionView
	for _, it := range items {
		if it.Session.Kind == registry.KindShadow {
			shadows = append(shadows, it)
		}
	}
	ti := textinput.New()
	ti.Placeholder = "command (e.g. npm run dev)"
	ti.CharLimit = 200
	var projects []string
	for n := range svc.Cfg.Projects {
		projects = append(projects, n)
	}
	m := shadowsModel{svc: svc, items: shadows, input: ti, projects: projects, vp: viewport.New(80, 20)}
	m.applyFilter()
	return m, nil
}

func (m *shadowsModel) applyFilter() {
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

func (m shadowsModel) current() (core.SessionView, bool) {
	if len(m.filtered) == 0 || m.cursor < 0 || m.cursor >= len(m.filtered) {
		return core.SessionView{}, false
	}
	return m.filtered[m.cursor], true
}

func (m shadowsModel) Init() tea.Cmd { return nil }

func (m shadowsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.vp.Width = max(20, msg.Width-2)
		m.vp.Height = max(5, msg.Height-6)
		return m, nil
	case tickMsg:
		if m.mode == shadowPeek {
			out, err := m.svc.Peek(m.peekRef)
			if err != nil {
				m.vp.SetContent(err.Error())
			} else {
				m.vp.SetContent(out)
				m.vp.GotoBottom()
			}
			return m, peekTick()
		}
		return m, nil
	case tea.KeyMsg:
		switch m.mode {
		case shadowPeek:
			return m.updatePeek(msg)
		case shadowConfirm:
			return m.updateConfirm(msg)
		case shadowPickProject:
			return m.updatePickProject(msg)
		case shadowCommand:
			return m.updateCommand(msg)
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

func (m shadowsModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		m.peekRef = it.Session.Project + "/" + it.Session.Name
		out, err := m.svc.Peek(m.peekRef)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.vp.SetContent(out)
		m.vp.GotoBottom()
		m.mode = shadowPeek
		return m, peekTick()
	case "R":
		it, ok := m.current()
		if !ok {
			return m, nil
		}
		if err := m.svc.Restart(it.Session.Project + "/" + it.Session.Name); err != nil {
			m.err = err.Error()
		}
		return m.reload()
	case "x":
		it, ok := m.current()
		if !ok {
			return m, nil
		}
		m.confirm = it.Session.Project + "/" + it.Session.Name
		m.mode = shadowConfirm
	case "o":
		it, ok := m.current()
		if !ok {
			return m, nil
		}
		if err := m.svc.Promote(it.Session.Project + "/" + it.Session.Name); err != nil {
			m.err = err.Error()
		}
		return m.reload()
	case "a":
		if len(m.projects) == 0 {
			m.err = "no projects configured"
			return m, nil
		}
		m.mode = shadowPickProject
		m.pCursor = 0
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

func (m shadowsModel) updatePeek(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.mode = shadowList
		return m, nil
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m shadowsModel) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		if err := m.svc.Kill(m.confirm); err != nil {
			m.err = err.Error()
		}
		m.mode = shadowList
		return m.reload()
	case "n", "esc", "q":
		m.mode = shadowList
	}
	return m, nil
}

func (m shadowsModel) updatePickProject(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.mode = shadowList
	case "up", "k":
		if m.pCursor > 0 {
			m.pCursor--
		}
	case "down", "j":
		if m.pCursor < len(m.projects)-1 {
			m.pCursor++
		}
	case "enter":
		m.mode = shadowCommand
		proj := m.projects[m.pCursor]
		if cmds := m.svc.Cfg.DefaultShadowCommands[proj]; len(cmds) > 0 {
			m.input.SetValue(cmds[0])
		} else {
			m.input.SetValue("")
		}
		m.input.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m shadowsModel) updateCommand(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = shadowList
		m.input.Blur()
		return m, nil
	case "enter":
		cmd := core.ResolveArgv(nil, m.input.Value())
		proj := m.projects[m.pCursor]
		if _, err := m.svc.NewShadow(proj, "", cmd); err != nil {
			m.err = err.Error()
		}
		m.mode = shadowList
		m.input.Blur()
		return m.reload()
	}
	var c tea.Cmd
	m.input, c = m.input.Update(msg)
	return m, c
}

func (m shadowsModel) reload() (tea.Model, tea.Cmd) {
	items, err := m.svc.List(true)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.items = nil
	for _, it := range items {
		if it.Session.Kind == registry.KindShadow {
			m.items = append(m.items, it)
		}
	}
	m.applyFilter()
	return m, nil
}

func (m shadowsModel) View() string {
	var b strings.Builder
	if m.mode == shadowPeek {
		b.WriteString(renderHeader("peek "+m.peekRef, "live · 1s", m.width) + "\n")
		b.WriteString(m.vp.View() + "\n")
		b.WriteString(renderHelp([]string{"esc/q back"}, m.width))
		return b.String()
	}
	b.WriteString(renderHeader("devhub shadows", "background processes", m.width) + "\n")
	if m.filter != "" {
		b.WriteString(mutedStyle.Render("filter: "+m.filter) + "\n")
	}
	if m.mode == shadowConfirm {
		b.WriteString(errStyle.Render("kill "+m.confirm+"? [y/n]") + "\n")
	}
	if m.mode == shadowPickProject {
		b.WriteString(mutedStyle.Render("project:") + "\n")
		for i, p := range m.projects {
			cur := "  "
			st := lipgloss.NewStyle()
			if i == m.pCursor {
				cur = "▸ "
				st = selStyle
			}
			b.WriteString(st.Render(cur+p) + "\n")
		}
	} else if m.mode == shadowCommand {
		b.WriteString("command: " + m.input.View() + "\n")
	} else {
		if len(m.filtered) == 0 {
			b.WriteString(mutedStyle.Render("  no shadow processes") + "\n")
		}
		for i, it := range m.filtered {
			s := it.Session
			cur := "  "
			st := rowStyle(s.Kind, i == m.cursor)
			if i == m.cursor {
				cur = "▸ "
			}
			stat := statusStyle(s.Status).Render(fmt.Sprintf("%-7s", s.Status))
			up := time.Since(s.CreatedAt).Truncate(time.Second)
			line := fmt.Sprintf("%s%-12s %-18s %s  up %s  rst %d  cpu %s%%  %s  %s",
				cur, s.Project, s.Name, stat, up, s.RestartCount, formatCPUPct(it.CPUPct), formatRSS(it.RSSBytes), s.Command)
			b.WriteString(st.Render(line) + "\n")
		}
	}
	b.WriteString("\n" + renderError(m.err) + "\n")
	b.WriteString(renderHelp([]string{"enter peek", "R restart", "x kill", "a add", "o promote", "esc/q close"}, m.width))
	return b.String()
}
