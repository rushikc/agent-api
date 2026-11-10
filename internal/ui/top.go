package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/monitor"
)

func RunTop(svc *core.Service) error {
	m := newTop(svc)
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

type topTick time.Time

func topTickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return topTick(t) })
}

type topModel struct {
	svc    *core.Service
	table  table.Model
	items  []core.SessionView
	sort   string
	filter string
	err    string
	width  int
	height int
}

func newTop(svc *core.Service) topModel {
	cols := []table.Column{
		{Title: "NAME", Width: 16},
		{Title: "PROJECT", Width: 12},
		{Title: "KIND", Width: 8},
		{Title: "PIDS", Width: 18},
		{Title: "CPU%", Width: 8},
		{Title: "RSS", Width: 10},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(12),
	)
	st := table.DefaultStyles()
	st.Header = st.Header.Bold(true).Foreground(colorAccent)
	st.Selected = st.Selected.Foreground(colorAccent).Bold(true)
	t.SetStyles(st)
	m := topModel{svc: svc, table: t, sort: "cpu"}
	m.refresh()
	return m
}

func (m *topModel) refresh() {
	items, err := m.svc.List(true)
	if err != nil {
		m.err = err.Error()
		return
	}
	m.items = items
	m.apply()
}

func (m *topModel) apply() {
	type row struct {
		v    core.SessionView
		pids string
	}
	var rows []row
	var totalCPU float64
	var totalRSS uint64
	for _, it := range m.items {
		h := sessionHaystack(it)
		if !filterMatch(m.filter, h) {
			continue
		}
		pids := make([]string, 0, len(it.PIDs))
		for _, p := range it.PIDs {
			pids = append(pids, fmt.Sprintf("%d", p))
		}
		rows = append(rows, row{v: it, pids: strings.Join(pids, ",")})
		if it.Session.Status == "running" {
			if it.CPUPct >= 0 {
				totalCPU += it.CPUPct
			}
			totalRSS += it.RSSBytes
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i].v, rows[j].v
		if m.sort == "mem" {
			if a.RSSBytes == b.RSSBytes {
				return a.Session.Name < b.Session.Name
			}
			return a.RSSBytes > b.RSSBytes
		}
		if a.CPUPct == b.CPUPct {
			return a.Session.Name < b.Session.Name
		}
		return a.CPUPct > b.CPUPct
	})
	trows := make([]table.Row, 0, len(rows)+1)
	for _, r := range rows {
		s := r.v.Session
		trows = append(trows, table.Row{
			s.Name,
			s.Project,
			string(s.Kind),
			r.pids,
			monitor.FormatCPUPct(r.v.CPUPct),
			formatRSS(r.v.RSSBytes),
		})
	}
	trows = append(trows, table.Row{"TOTAL", "", "", "", fmt.Sprintf("%.1f", totalCPU), formatRSS(totalRSS)})
	m.table.SetRows(trows)
}

func (m topModel) Init() tea.Cmd { return topTickCmd() }

func (m topModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.table.SetHeight(max(5, msg.Height-6))
		return m, nil
	case topTick:
		m.refresh()
		return m, topTickCmd()
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			return m, tea.Quit
		case "c":
			m.sort = "cpu"
			m.apply()
		case "m":
			m.sort = "mem"
			m.apply()
		case "backspace":
			if m.filter != "" {
				m.filter = m.filter[:len(m.filter)-1]
				m.apply()
			}
		default:
			if len(msg.Runes) == 1 && msg.Type == tea.KeyRunes {
				m.filter += string(msg.Runes)
				m.apply()
			} else {
				var cmd tea.Cmd
				m.table, cmd = m.table.Update(msg)
				return m, cmd
			}
		}
	}
	return m, nil
}

func (m topModel) View() string {
	var b strings.Builder
	b.WriteString(renderHeader("devhub top", "sort "+m.sort+" · 2s", m.width) + "\n")
	if m.filter != "" {
		b.WriteString(mutedStyle.Render("filter: "+m.filter) + "\n")
	}
	b.WriteString(m.table.View() + "\n")
	b.WriteString(renderError(m.err) + "\n")
	b.WriteString(renderHelp([]string{"c cpu", "m memory", "type to filter", "esc/q close"}, m.width))
	return lipgloss.NewStyle().Render(b.String())
}
