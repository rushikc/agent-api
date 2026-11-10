package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rushikesh/devhub/internal/client"
	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/daemon"
	"github.com/rushikesh/devhub/internal/registry"
)

func RunAlerts() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	_ = daemon.Ensure(cfg)
	c := client.Unix(cfg, os.Getenv("DEVHUB_CALLER"))
	m := alertsModel{c: c}
	m.refresh()
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

type alertsModel struct {
	c     *client.Client
	rows  []registry.EventRow
	cur   int
	width int
	err   string
}

type alertsTick struct{}

func (m alertsModel) Init() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return alertsTick{} })
}

func (m *alertsModel) refresh() {
	b, err := m.c.Alerts()
	if err != nil {
		m.err = err.Error()
		return
	}
	_ = json.Unmarshal(b, &m.rows)
}

func (m alertsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case alertsTick:
		m.refresh()
		return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return alertsTick{} })
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cur > 0 {
				m.cur--
			}
		case "down", "j":
			if m.cur < len(m.rows)-1 {
				m.cur++
			}
		}
	}
	return m, nil
}

func (m alertsModel) View() string {
	s := renderHeader("Alerts", "recent events", m.width) + "\n"
	for i, r := range m.rows {
		line := fmt.Sprintf("%s  %s  %s  %s", r.TS.Format(time.Kitchen), r.Type, r.Target, r.Project)
		if i == m.cur {
			line = selStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		s += line + "\n"
	}
	s += renderError(m.err)
	s += "\n" + renderHelp([]string{"esc quit"}, m.width)
	return s
}
