package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/skills"
)

func RunSkills() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	hub, err := skills.Open(cfg)
	if err != nil {
		return err
	}
	items, err := hub.List()
	if err != nil {
		return err
	}
	m := skillsModel{hub: hub, items: items}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

type skillsModel struct {
	hub      *skills.Hub
	items    []skills.Item
	filtered []skills.Item
	cursor   int
	filter   string
	err      string
	width    int
	height   int
}

func (m *skillsModel) apply() {
	m.filtered = nil
	for _, it := range m.items {
		h := it.Kind + " " + it.Name + " " + it.Scope
		if filterMatch(m.filter, h) {
			m.filtered = append(m.filtered, it)
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m skillsModel) Init() tea.Cmd { m.apply(); return nil }

func (m skillsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.apply()
	case tea.KeyMsg:
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
		case "c":
			if it, ok := m.current(); ok {
				cmd := exec.Command("pbcopy")
				cmd.Stdin = strings.NewReader(it.Body)
				_ = cmd.Run()
			}
		case "e":
			if it, ok := m.current(); ok {
				ed := os.Getenv("EDITOR")
				if ed == "" {
					ed = "nano"
				}
				c := exec.Command(ed, it.Path)
				c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
				_ = c.Run()
			}
		case "s":
			_, err := m.hub.Sync(false)
			if err != nil {
				m.err = err.Error()
			}
		case "d":
			iss := m.hub.Doctor()
			m.err = fmt.Sprintf("%d doctor issues", len(iss))
		case "t":
			m.err = "toggle: use CLI skills enable|disable --project"
		case "backspace":
			if m.filter != "" {
				m.filter = m.filter[:len(m.filter)-1]
				m.apply()
			}
		default:
			if len(msg.Runes) == 1 && msg.Type == tea.KeyRunes {
				m.filter += string(msg.Runes)
				m.apply()
			}
		}
	}
	return m, nil
}

func (m skillsModel) current() (skills.Item, bool) {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return skills.Item{}, false
	}
	return m.filtered[m.cursor], true
}

func (m skillsModel) View() string {
	m.apply()
	var left strings.Builder
	left.WriteString(renderHeader("skills / rules", "", m.width/2) + "\n")
	if m.filter != "" {
		left.WriteString(mutedStyle.Render("filter: "+m.filter) + "\n")
	}
	for i, it := range m.filtered {
		cur := "  "
		st := lipgloss.NewStyle()
		if i == m.cursor {
			cur = "▸ "
			st = selStyle
		}
		left.WriteString(st.Render(fmt.Sprintf("%s%s %-16s %s", cur, it.Kind, it.Name, it.Scope)) + "\n")
	}
	preview := mutedStyle.Render("select an item")
	if it, ok := m.current(); ok {
		r, err := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(max(20, m.width/2-2)))
		if err == nil {
			if out, err := r.Render(it.Body); err == nil {
				preview = out
			} else {
				preview = it.Body
			}
		} else {
			preview = it.Body
		}
	}
	split := lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(max(20, m.width/2)).Render(left.String()), preview)
	return split + "\n" + renderError(m.err) + "\n" + renderHelp([]string{"c copy", "e edit", "t toggle", "s sync", "d doctor", "esc/q"}, m.width)
}
