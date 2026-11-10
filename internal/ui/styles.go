package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/registry"
)

var (
	colorTitle  = lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#E6E6E6"}
	colorMuted  = lipgloss.AdaptiveColor{Light: "#6B6B6B", Dark: "#7A7A7A"}
	colorGreen  = lipgloss.AdaptiveColor{Light: "#147A3F", Dark: "#7DCE9A"}
	colorRed    = lipgloss.AdaptiveColor{Light: "#B00020", Dark: "#FF6B6B"}
	colorAccent = lipgloss.AdaptiveColor{Light: "#3B6D99", Dark: "#89B4FA"}
	colorHelp   = lipgloss.AdaptiveColor{Light: "#5A5A5A", Dark: "#9A9A9A"}

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorTitle)
	mutedStyle = lipgloss.NewStyle().Foreground(colorMuted)
	dimStyle   = lipgloss.NewStyle().Faint(true).Foreground(colorMuted)
	runStyle   = lipgloss.NewStyle().Foreground(colorGreen)
	exitStyle  = lipgloss.NewStyle().Foreground(colorRed)
	selStyle   = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	helpStyle  = lipgloss.NewStyle().Foreground(colorHelp)
	errStyle   = lipgloss.NewStyle().Foreground(colorRed)
	headerBox  = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(colorAccent)
)

func statusStyle(st registry.Status) lipgloss.Style {
	if st == registry.StatusExited {
		return exitStyle
	}
	return runStyle
}

func rowStyle(kind registry.Kind, selected bool) lipgloss.Style {
	st := lipgloss.NewStyle()
	if kind == registry.KindShadow {
		st = dimStyle
	}
	if selected {
		st = selStyle
	}
	return st
}

func renderHeader(title, subtitle string, width int) string {
	line := headerBox.Render(title)
	if subtitle != "" {
		line += "  " + mutedStyle.Render(subtitle)
	}
	if width > 0 {
		return lipgloss.NewStyle().Width(width).Render(line)
	}
	return line
}

func renderHelp(keys []string, width int) string {
	s := helpStyle.Render(strings.Join(keys, "  ·  "))
	if width > 0 {
		return lipgloss.NewStyle().Width(width).Render(s)
	}
	return s
}

func renderError(err string) string {
	if err == "" {
		return ""
	}
	return errStyle.Render("! " + err)
}

func filterMatch(filter, haystack string) bool {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return true
	}
	return strings.Contains(strings.ToLower(haystack), filter)
}

func fuzzyMatch(filter, haystack string) bool {
	f := []rune(strings.ToLower(strings.TrimSpace(filter)))
	if len(f) == 0 {
		return true
	}
	h := []rune(strings.ToLower(haystack))
	j := 0
	for i := 0; i < len(h) && j < len(f); i++ {
		if h[i] == f[j] {
			j++
		}
	}
	return j == len(f)
}

func sessionHaystack(v core.SessionView) string {
	s := v.Session
	return strings.Join([]string{s.Project, s.Name, string(s.Kind), string(s.Status), s.Command}, " ")
}

func formatRSS(n uint64) string {
	if n >= 1024*1024*1024 {
		return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
	}
	if n >= 1024*1024 {
		return fmt.Sprintf("%.0f MB", float64(n)/(1024*1024))
	}
	if n >= 1024 {
		return fmt.Sprintf("%.0f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%d B", n)
}
