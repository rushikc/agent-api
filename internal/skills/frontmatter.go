package skills

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type SkillMeta struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Scope       string   `yaml:"scope"` // global | project:name
	Targets     []string `yaml:"targets"`
	Enabled     *bool    `yaml:"enabled,omitempty"`
}

type RuleMeta struct {
	Description string   `yaml:"description"`
	Scope       string   `yaml:"scope"`
	Globs       string   `yaml:"globs"`
	AlwaysApply bool     `yaml:"alwaysApply"`
	Targets     []string `yaml:"targets"`
}

func SplitFrontmatter(body string) (map[string]any, string, error) {
	body = strings.TrimPrefix(body, "\ufeff")
	if !strings.HasPrefix(body, "---") {
		return map[string]any{}, body, nil
	}
	rest := strings.TrimPrefix(body, "---")
	rest = strings.TrimLeft(rest, "\r\n")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, "", fmt.Errorf("unclosed YAML frontmatter")
	}
	raw := rest[:idx]
	content := strings.TrimLeft(rest[idx+len("\n---"):], "\r\n")
	meta := map[string]any{}
	if err := yaml.Unmarshal([]byte(raw), &meta); err != nil {
		return nil, "", fmt.Errorf("frontmatter: %w", err)
	}
	return meta, content, nil
}

func ParseSkill(body string) (SkillMeta, string, error) {
	fm, content, err := SplitFrontmatter(body)
	if err != nil {
		return SkillMeta{}, "", err
	}
	b, _ := yaml.Marshal(fm)
	var m SkillMeta
	if err := yaml.Unmarshal(b, &m); err != nil {
		return SkillMeta{}, "", err
	}
	if m.Name == "" {
		return SkillMeta{}, "", fmt.Errorf("skill frontmatter missing name")
	}
	if m.Scope == "" {
		m.Scope = "global"
	}
	return m, content, nil
}

func ParseRule(body string) (RuleMeta, string, error) {
	fm, content, err := SplitFrontmatter(body)
	if err != nil {
		return RuleMeta{}, "", err
	}
	b, _ := yaml.Marshal(fm)
	var m RuleMeta
	if err := yaml.Unmarshal(b, &m); err != nil {
		return RuleMeta{}, "", err
	}
	if m.Scope == "" {
		m.Scope = "global"
	}
	return m, content, nil
}

func ToMDC(meta RuleMeta, content string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("description: " + yamlScalar(meta.Description) + "\n")
	if meta.Globs != "" {
		b.WriteString("globs: " + yamlScalar(meta.Globs) + "\n")
	}
	if meta.AlwaysApply {
		b.WriteString("alwaysApply: true\n")
	} else {
		b.WriteString("alwaysApply: false\n")
	}
	b.WriteString("---\n")
	b.WriteString(ManagedHeader)
	b.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		b.WriteByte('\n')
	}
	return b.String()
}

func yamlScalar(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, ":#\n\"'") {
		b, _ := yaml.Marshal(s)
		return strings.TrimSpace(string(b))
	}
	return s
}

const ManagedHeader = "<!-- managed by devhub, edit in ~/.devhub/skills -->\n"
const ManagedBlockStart = "<!-- BEGIN DEVHUB -->\n"
const ManagedBlockEnd = "<!-- END DEVHUB -->\n"
