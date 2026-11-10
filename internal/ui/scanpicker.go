package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/scan"
)

type scanTab int

const (
	scanTabAll scanTab = iota
	scanTabAdded
	scanTabNotAdded
)

type scanSort int

const (
	scanSortName scanSort = iota
	scanSortActivity
	scanSortPath
)

type scanMode int

const (
	scanList scanMode = iota
	scanConfirm
	scanKill
	scanEdit
	scanBusy
	scanToast
)

type scanRow struct {
	Repo        scan.Repo
	Checked     bool
	OrigChecked bool
	AddName     string
}

type scanApplyDiff struct {
	Adds    []scanApplyAdd
	Removes []string
	Renames []scanApplyRename
}

type scanApplyAdd struct {
	Name string
	Path string
}

type scanApplyRename struct {
	Old string
	New string
}

type scanModel struct {
	svc       *core.Service
	rows      []scanRow
	cursor    int
	filter    string
	tab       scanTab
	sort      scanSort
	mode      scanMode
	err       string
	width     int
	height    int
	roots     []string
	cacheAge  time.Duration
	fromCache bool
	denied    int
	privacy   bool
	input     textinput.Model
	toast     string
	summary   string
	parent    tea.Model
	killAll   bool
}

type scanDoneMsg struct {
	res *scan.Result
	err error
	all bool
}

type toastExpiredMsg struct{}

func RunScanPicker(svc *core.Service, res *scan.Result, cacheAge time.Duration, fromCache bool) error {
	m := newScanModel(svc, res, cacheAge, fromCache)
	out, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}
	if sm, ok := out.(scanModel); ok && sm.summary != "" {
		fmt.Println(sm.summary)
	}
	return nil
}

func newScanModel(svc *core.Service, res *scan.Result, cacheAge time.Duration, fromCache bool) scanModel {
	ti := textinput.New()
	ti.CharLimit = 48
	m := scanModel{svc: svc, input: ti, cacheAge: cacheAge, fromCache: fromCache}
	m.loadResult(res)
	return m
}

func (m *scanModel) loadResult(res *scan.Result) {
	if res == nil {
		m.rows = nil
		return
	}
	m.roots = append([]string(nil), res.Roots...)
	m.denied = res.Denied
	m.privacy = res.PrivacyDenied
	projects := map[string]string{}
	if m.svc != nil && m.svc.Cfg != nil {
		projects = m.svc.Cfg.Projects
		scan.Annotate(res, projects) // registered names
	}
	m.rows = make([]scanRow, 0, len(res.Repos))
	for _, r := range res.Repos {
		name := r.Name
		if r.Registered && r.RegisteredName != "" {
			name = r.RegisteredName
		} else if m.svc != nil && m.svc.Cfg != nil {
			name = m.svc.Cfg.UniqueProjectName(r.Name)
		}
		m.rows = append(m.rows, scanRow{
			Repo:        r,
			Checked:     r.Registered,
			OrigChecked: r.Registered,
			AddName:     name,
		})
	}
	m.applySort()
	m.cursor = 0
}

func (m *scanModel) applySort() {
	switch m.sort {
	case scanSortActivity:
		sort.SliceStable(m.rows, func(i, j int) bool {
			return m.rows[i].Repo.LastActivity.After(m.rows[j].Repo.LastActivity)
		})
	case scanSortPath:
		sort.SliceStable(m.rows, func(i, j int) bool {
			return m.rows[i].Repo.Path < m.rows[j].Repo.Path
		})
	default:
		sort.SliceStable(m.rows, func(i, j int) bool {
			return strings.ToLower(m.rows[i].AddName) < strings.ToLower(m.rows[j].AddName)
		})
	}
}

func (m scanModel) visible() []int {
	var out []int
	for i, r := range m.rows {
		switch m.tab {
		case scanTabAdded:
			if !r.OrigChecked {
				continue
			}
		case scanTabNotAdded:
			if r.OrigChecked {
				continue
			}
		}
		hay := r.AddName + " " + r.Repo.Path + " " + r.Repo.Branch + " " + strings.Join(r.Repo.Stack, " ")
		if !fuzzyMatch(m.filter, hay) {
			continue
		}
		out = append(out, i)
	}
	return out
}

func (m *scanModel) toggleIndex(i int) {
	if i < 0 || i >= len(m.rows) {
		return
	}
	m.rows[i].Checked = !m.rows[i].Checked
}

func (m *scanModel) toggleAllVisible() {
	vis := m.visible()
	allOn := true
	for _, i := range vis {
		if !m.rows[i].Checked {
			allOn = false
			break
		}
	}
	for _, i := range vis {
		m.rows[i].Checked = !allOn
	}
}

func (m scanModel) applyDiff() scanApplyDiff {
	var d scanApplyDiff
	reserved := map[string]string{}
	for _, r := range m.rows {
		if r.Checked && !r.OrigChecked {
			reserved[r.AddName] = r.Repo.Path
		}
	}
	projects := map[string]string{}
	if m.svc != nil && m.svc.Cfg != nil {
		projects = m.svc.Cfg.Projects
	}
	used := map[string]string{}
	for _, r := range m.rows {
		if r.Checked && !r.OrigChecked {
			name := r.AddName
			if name == "" {
				name = r.Repo.Name
			}
			name = core.UniqueAddName(projects, name, used)
			used[name] = r.Repo.Path
			d.Adds = append(d.Adds, scanApplyAdd{Name: name, Path: r.Repo.Path})
		}
		if !r.Checked && r.OrigChecked && r.Repo.RegisteredName != "" {
			d.Removes = append(d.Removes, r.Repo.RegisteredName)
		}
		if r.Checked && r.OrigChecked && r.Repo.RegisteredName != "" && r.AddName != r.Repo.RegisteredName {
			d.Renames = append(d.Renames, scanApplyRename{Old: r.Repo.RegisteredName, New: r.AddName})
		}
	}
	return d
}

func (m scanModel) Init() tea.Cmd { return nil }

func (m scanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case toastExpiredMsg:
		return m.quit()
	case scanDoneMsg:
		m.mode = scanList
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.fromCache = false
		m.cacheAge = 0
		m.loadResult(msg.res)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m scanModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == scanBusy {
		if msg.String() == "ctrl+c" {
			return m.quit()
		}
		return m, nil
	}
	if m.mode == scanToast {
		if msg.String() == "esc" || msg.String() == "enter" || msg.String() == "q" {
			return m.quit()
		}
		return m, nil
	}
	if m.mode == scanEdit {
		switch msg.String() {
		case "esc":
			m.mode = scanList
			return m, nil
		case "enter":
			vis := m.visible()
			if m.cursor >= 0 && m.cursor < len(vis) {
				name := strings.TrimSpace(m.input.Value())
				if name != "" {
					m.rows[vis[m.cursor]].AddName = name
				}
			}
			m.mode = scanList
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	if m.mode == scanConfirm {
		switch msg.String() {
		case "esc", "n", "q":
			m.mode = scanList
		case "enter", "y":
			diff := m.applyDiff()
			if m.removesHaveSessions(diff) {
				m.mode = scanKill
				m.killAll = false
				return m, nil
			}
			return m.doApply(diff, false)
		}
		return m, nil
	}
	if m.mode == scanKill {
		switch msg.String() {
		case "esc", "n", "q":
			m.mode = scanList
		case "enter":
			return m.doApply(m.applyDiff(), false)
		case "k":
			return m.doApply(m.applyDiff(), true)
		}
		return m, nil
	}
	vis := m.visible()
	switch msg.String() {
	case "esc", "q", "ctrl+c":
		return m.quit()
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(vis)-1 {
			m.cursor++
		}
	case " ":
		if m.cursor >= 0 && m.cursor < len(vis) {
			m.toggleIndex(vis[m.cursor])
		}
	case "a":
		m.toggleAllVisible()
	case "enter":
		diff := m.applyDiff()
		if len(diff.Adds)+len(diff.Removes)+len(diff.Renames) == 0 {
			m.err = "no changes"
			return m, nil
		}
		m.mode = scanConfirm
		m.err = ""
	case "e":
		if m.cursor >= 0 && m.cursor < len(vis) {
			m.input.SetValue(m.rows[vis[m.cursor]].AddName)
			m.input.Focus()
			m.mode = scanEdit
			return m, textinput.Blink
		}
	case "tab":
		m.tab = (m.tab + 1) % 3
		m.cursor = 0
	case "s":
		m.sort = (m.sort + 1) % 3
		m.applySort()
	case "o":
		if row, ok := m.current(vis); ok {
			_ = exec.Command("open", row.Repo.Path).Start()
		}
	case "c":
		if row, ok := m.current(vis); ok {
			_ = exec.Command("cursor", row.Repo.Path).Start()
		}
	case "r":
		m.mode = scanBusy
		return m, m.rescan(false)
	case "R":
		m.mode = scanBusy
		return m, m.rescan(true)
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
	return m, nil
}

func (m scanModel) current(vis []int) (scanRow, bool) {
	if m.cursor < 0 || m.cursor >= len(vis) {
		return scanRow{}, false
	}
	return m.rows[vis[m.cursor]], true
}

func (m scanModel) removesHaveSessions(d scanApplyDiff) bool {
	if m.svc == nil {
		return false
	}
	for _, name := range d.Removes {
		if m.svc.ProjectHasSessions(name) {
			return true
		}
	}
	return false
}

func (m scanModel) doApply(d scanApplyDiff, kill bool) (tea.Model, tea.Cmd) {
	if m.svc == nil {
		m.err = "no service"
		m.mode = scanList
		return m, nil
	}
	var added, removed, renamed int
	for _, a := range d.Adds {
		if err := m.svc.AddProject(a.Name, a.Path); err != nil {
			m.err = err.Error()
			m.mode = scanList
			return m, nil
		}
		added++
	}
	for _, name := range d.Removes {
		if err := m.svc.RemoveProject(name, kill); err != nil {
			m.err = err.Error()
			m.mode = scanList
			return m, nil
		}
		removed++
	}
	for _, rn := range d.Renames {
		if err := m.svc.RenameProject(rn.Old, rn.New); err != nil {
			m.err = err.Error()
			m.mode = scanList
			return m, nil
		}
		renamed++
	}
	m.summary = fmt.Sprintf("added %d, removed %d, renamed %d", added, removed, renamed)
	m.toast = m.summary
	m.mode = scanToast
	return m, tea.Tick(1200*time.Millisecond, func(time.Time) tea.Msg { return toastExpiredMsg{} })
}

func (m scanModel) quit() (tea.Model, tea.Cmd) {
	if m.parent != nil {
		if p, ok := m.parent.(projectsModel); ok {
			p.reload()
			p.width, p.height = m.width, m.height
			return p, nil
		}
		return m.parent, nil
	}
	return m, tea.Quit
}

func (m scanModel) rescan(all bool) tea.Cmd {
	svc := m.svc
	return func() tea.Msg {
		cfg := svc.Cfg
		vols := false
		if cfg != nil {
			vols = cfg.Scan.IncludeVolumes
		}
		roots := scan.ResolveRoots(cfg, all, nil, vols)
		depth := scan.ResolveDepth(cfg, all, 0)
		exclude := []string{}
		projects := map[string]string{}
		if cfg != nil {
			exclude = cfg.Scan.Exclude
			projects = cfg.Projects
		}
		hidden := false
		if cfg != nil {
			hidden = cfg.Scan.IncludeHidden
		}
		res, err := scan.Walk(context.Background(), scan.Options{
			Roots: roots, Depth: depth, IncludeVolumes: vols, IncludeHidden: hidden,
			Exclude: exclude, Projects: projects, AllowRoots: scan.ExtraAllowRoots(cfg, nil),
		})
		if err == nil && cfg != nil {
			_ = scan.SaveCache(cfg.ScanCachePath(), scan.CacheFromResult(res))
		}
		return scanDoneMsg{res: res, err: err, all: all}
	}
}

func (m scanModel) View() string {
	var b strings.Builder
	age := "just now"
	if m.cacheAge > 0 {
		age = formatRel(m.cacheAge) + " ago"
		if m.fromCache {
			age += " (cache)"
		}
	}
	root := strings.Join(shortenAll(m.roots), ", ")
	if root == "" {
		root = "~/Desktop"
	}
	b.WriteString(renderHeader("scan projects", fmt.Sprintf("%s  %d repos  %s", root, len(m.rows), age), m.width) + "\n")
	if m.privacy {
		b.WriteString(errStyle.Render(scan.PrivacyHint()) + "\n")
	}
	if m.mode == scanBusy {
		b.WriteString(mutedStyle.Render("scanning…") + "\n")
		return b.String()
	}
	if m.mode == scanToast {
		b.WriteString(runStyle.Render("✓ "+m.toast) + "\n")
		return b.String()
	}
	if m.mode == scanConfirm {
		d := m.applyDiff()
		b.WriteString(errStyle.Render(fmt.Sprintf("Add %d, remove %d, rename %d? [enter confirm / esc]", len(d.Adds), len(d.Removes), len(d.Renames))) + "\n")
		return b.String()
	}
	if m.mode == scanKill {
		b.WriteString(errStyle.Render("Removed projects have running sessions. enter=keep  k=kill  esc=cancel") + "\n")
		return b.String()
	}
	if m.filter != "" {
		b.WriteString(mutedStyle.Render("filter: "+m.filter) + "\n")
	}
	tabs := []string{"all", "added", "not added"}
	tabs[m.tab] = "[" + tabs[m.tab] + "]"
	b.WriteString(mutedStyle.Render(strings.Join(tabs, "  ")+"  sort:"+[]string{"name", "activity", "path"}[m.sort]) + "\n")
	if m.mode == scanEdit {
		b.WriteString("name: " + m.input.View() + "\n")
	}
	vis := m.visible()
	listW := m.width * 3 / 5
	if listW < 40 {
		listW = m.width
	}
	var list strings.Builder
	if len(vis) == 0 {
		list.WriteString(mutedStyle.Render("  no repositories") + "\n")
	}
	maxRows := m.height - 8
	if maxRows < 5 {
		maxRows = 12
	}
	start := 0
	if m.cursor >= maxRows {
		start = m.cursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(vis) {
		end = len(vis)
	}
	for n, vi := range vis {
		if n < start || n >= end {
			continue
		}
		r := m.rows[vi]
		mark := "[ ]"
		if r.Checked {
			mark = "[x]"
		}
		added := ""
		if r.OrigChecked {
			added = " added"
		}
		line := fmt.Sprintf("%s %-16s  %s  %s  %s  %s%s", mark, r.AddName, shortenPath(r.Repo.Path), strings.Join(r.Repo.Stack, ","), r.Repo.Branch, formatRel(time.Since(r.Repo.LastActivity)), added)
		st := lipgloss.NewStyle()
		cur := "  "
		if n == m.cursor {
			cur = "▸ "
			st = selStyle
		}
		list.WriteString(st.Render(cur+line) + "\n")
	}
	preview := mutedStyle.Render("select a repo")
	if row, ok := m.current(vis); ok {
		preview = strings.Join([]string{
			titleStyle.Render(row.AddName),
			row.Repo.Path,
			"remote: " + emptyDash(row.Repo.Remote),
			"branch: " + emptyDash(row.Repo.Branch),
			"stack: " + emptyDash(strings.Join(row.Repo.Stack, ", ")),
			fmt.Sprintf("devhub.yaml: %v", row.Repo.HasDevhubYAML),
			fmt.Sprintf("registered: %v %s", row.Repo.Registered, row.Repo.RegisteredName),
		}, "\n")
	}
	if m.width > 80 {
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(listW).Render(list.String()), "  ", lipgloss.NewStyle().Width(m.width-listW-2).Render(preview)))
	} else {
		b.WriteString(list.String())
		b.WriteString("\n" + preview + "\n")
	}
	b.WriteString("\n" + renderError(m.err) + "\n")
	b.WriteString(renderHelp([]string{"space toggle", "a all", "enter apply", "e name", "tab filter", "s sort", "o finder", "c cursor", "r/R rescan", "esc/q"}, m.width))
	return b.String()
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func shortenPath(p string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && (p == home || strings.HasPrefix(p, home+"/")) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

func shortenAll(ps []string) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, shortenPath(p))
	}
	return out
}

func formatRel(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dmo", int(d.Hours()/24/30))
	}
}
