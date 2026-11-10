package skills

import (
	"os"
	"path/filepath"
	"strings"
)

type CursorAdapter struct{ Home string }

func (CursorAdapter) Name() string { return "cursor" }

func (a CursorAdapter) Sync(h *Hub, items []Item, dry bool) ([]string, error) {
	var wrote []string
	for _, it := range items {
		if !it.Enabled {
			continue
		}
		switch it.Kind {
		case "skill":
			src := filepath.Dir(it.Path)
			if it.Skill.Scope == "global" || it.Skill.Scope == "" {
				dst := filepath.Join(a.Home, ".cursor", "skills", it.Name)
				if err := linkOrCopy(src, dst, dry); err != nil {
					return wrote, err
				}
				wrote = append(wrote, dst)
			} else {
				_, repo := projectPath(h.Cfg, it.Skill.Scope)
				if repo == "" {
					continue
				}
				dst := filepath.Join(repo, ".cursor", "skills", it.Name)
				if err := linkOrCopy(src, dst, dry); err != nil {
					return wrote, err
				}
				wrote = append(wrote, dst)
			}
		case "rule":
			mdc := ToMDC(it.Rule, func() string {
				_, c, _ := ParseRule(it.Body)
				return c
			}())
			if it.Rule.Scope == "global" || it.Rule.Scope == "" {
				for _, repo := range h.Cfg.Projects {
					abs, err := filepath.Abs(repo)
					if err != nil {
						continue
					}
					dst := filepath.Join(abs, ".cursor", "rules", "dh-global-"+it.Name+".mdc")
					if err := writeManaged(dst, mdc, dry); err != nil {
						return wrote, err
					}
					wrote = append(wrote, dst)
				}
			} else {
				_, repo := projectPath(h.Cfg, it.Rule.Scope)
				if repo == "" {
					continue
				}
				dst := filepath.Join(repo, ".cursor", "rules", it.Name+".mdc")
				if err := writeManaged(dst, mdc, dry); err != nil {
					return wrote, err
				}
				wrote = append(wrote, dst)
			}
		}
	}
	return wrote, nil
}

func (a CursorAdapter) Unsync(h *Hub, manifest Manifest) error {
	for _, f := range manifest.Files {
		removeIfManaged(f, manifest)
	}
	return nil
}

func (a CursorAdapter) Doctor(h *Hub, items []Item, manifest Manifest) []Issue {
	var issues []Issue
	for _, f := range manifest.Files {
		st, err := os.Lstat(f)
		if err != nil {
			issues = append(issues, Issue{Kind: "orphan", Path: f, Message: "managed path missing"})
			continue
		}
		if st.Mode()&os.ModeSymlink != 0 {
			if _, err := os.Stat(f); err != nil {
				issues = append(issues, Issue{Kind: "broken-symlink", Path: f, Message: err.Error()})
			}
		}
	}
	return issues
}

type ClaudeAdapter struct{ Home string }

func (ClaudeAdapter) Name() string { return "claude" }

func (a ClaudeAdapter) Sync(h *Hub, items []Item, dry bool) ([]string, error) {
	var wrote []string
	var globalRules []string
	for _, it := range items {
		if it.Kind == "skill" && it.Enabled {
			src := filepath.Dir(it.Path)
			dst := filepath.Join(a.Home, ".claude", "skills", it.Name)
			if strings.HasPrefix(it.Skill.Scope, "project:") {
				_, repo := projectPath(h.Cfg, it.Skill.Scope)
				if repo != "" {
					dst = filepath.Join(repo, ".claude", "skills", it.Name)
				}
			}
			if err := linkOrCopy(src, dst, dry); err != nil {
				return wrote, err
			}
			wrote = append(wrote, dst)
		}
		if it.Kind == "rule" {
			_, content, _ := ParseRule(it.Body)
			globalRules = append(globalRules, "## "+it.Name+"\n"+content)
		}
	}
	if len(globalRules) > 0 {
		p := filepath.Join(a.Home, ".claude", "CLAUDE.md")
		if err := upsertBlock(p, strings.Join(globalRules, "\n"), dry); err != nil {
			return wrote, err
		}
		wrote = append(wrote, p)
	}
	return wrote, nil
}

func (a ClaudeAdapter) Unsync(h *Hub, m Manifest) error {
	for _, f := range m.Files {
		removeIfManaged(f, m)
	}
	return nil
}
func (a ClaudeAdapter) Doctor(h *Hub, items []Item, m Manifest) []Issue { return nil }

type CodexAdapter struct{ Home string }

func (CodexAdapter) Name() string { return "codex" }

func (a CodexAdapter) Sync(h *Hub, items []Item, dry bool) ([]string, error) {
	var rules []string
	for _, it := range items {
		if it.Kind != "rule" {
			continue
		}
		_, content, _ := ParseRule(it.Body)
		rules = append(rules, "## "+it.Name+"\n"+content)
	}
	p := filepath.Join(a.Home, ".codex", "AGENTS.md")
	if err := upsertBlock(p, strings.Join(rules, "\n"), dry); err != nil {
		return nil, err
	}
	return []string{p}, nil
}
func (a CodexAdapter) Unsync(h *Hub, m Manifest) error {
	for _, f := range m.Files {
		removeIfManaged(f, m)
	}
	return nil
}
func (a CodexAdapter) Doctor(h *Hub, items []Item, m Manifest) []Issue { return nil }

func upsertBlock(path, inner string, dry bool) error {
	if dry {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	block := ManagedBlockStart + ManagedHeader + inner + "\n" + ManagedBlockEnd
	existing, _ := os.ReadFile(path)
	s := string(existing)
	start := strings.Index(s, strings.TrimSpace(ManagedBlockStart))
	end := strings.Index(s, strings.TrimSpace(ManagedBlockEnd))
	if start >= 0 && end > start {
		end = end + len(strings.TrimSpace(ManagedBlockEnd))
		s = s[:start] + block + s[end:]
	} else {
		if s != "" && !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		s += "\n" + block
	}
	return os.WriteFile(path, []byte(s), 0o644)
}
