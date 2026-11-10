package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rushikesh/devhub/internal/config"
	"gopkg.in/yaml.v3"
)

type Item struct {
	Kind    string    `json:"kind"` // skill | rule
	Name    string    `json:"name"`
	Scope   string    `json:"scope"`
	Path    string    `json:"path"`
	Body    string    `json:"-"`
	Skill   SkillMeta `json:"skill,omitempty"`
	Rule    RuleMeta  `json:"rule,omitempty"`
	Enabled bool      `json:"enabled"`
}

type Manifest struct {
	Files []string `json:"files"`
}

type Hub struct {
	Root     string
	Cfg      *config.Config
	Home     string
	Adapters []Adapter
}

func Open(cfg *config.Config) (*Hub, error) {
	root := cfg.SkillsDir()
	h := &Hub{Root: root, Cfg: cfg, Home: userHome()}
	if err := h.ensureRepo(); err != nil {
		return nil, err
	}
	h.Adapters = enabledAdapters(cfg, h.Home)
	return h, nil
}

func userHome() string {
	h, _ := os.UserHomeDir()
	return h
}

func (h *Hub) ensureRepo() error {
	if err := os.MkdirAll(filepath.Join(h.Root, "skills"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(h.Root, "rules"), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(h.Root, ".git")); err != nil {
		cmd := exec.Command("git", "init")
		cmd.Dir = h.Root
		_ = cmd.Run()
	}
	return nil
}

func (h *Hub) commit(msg string) {
	_ = exec.Command("git", "-C", h.Root, "add", "-A").Run()
	_ = exec.Command("git", "-C", h.Root, "commit", "-m", msg).Run()
}

func (h *Hub) List() ([]Item, error) {
	var items []Item
	sk, _ := os.ReadDir(filepath.Join(h.Root, "skills"))
	for _, e := range sk {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(h.Root, "skills", e.Name(), "SKILL.md")
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		m, _, err := ParseSkill(string(b))
		if err != nil {
			continue
		}
		en := true
		if m.Enabled != nil {
			en = *m.Enabled
		}
		items = append(items, Item{Kind: "skill", Name: m.Name, Scope: m.Scope, Path: p, Body: string(b), Skill: m, Enabled: en})
	}
	ru, _ := os.ReadDir(filepath.Join(h.Root, "rules"))
	for _, e := range ru {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		p := filepath.Join(h.Root, "rules", e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		m, _, err := ParseRule(string(b))
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		items = append(items, Item{Kind: "rule", Name: name, Scope: m.Scope, Path: p, Body: string(b), Rule: m, Enabled: true})
	}
	return items, nil
}

func (h *Hub) Get(name string) (Item, error) {
	items, err := h.List()
	if err != nil {
		return Item{}, err
	}
	for _, it := range items {
		if it.Name == name {
			return it, nil
		}
	}
	return Item{}, fmt.Errorf("unknown skill/rule %s", name)
}

func (h *Hub) SetEnabled(name string, enabled bool) error {
	it, err := h.Get(name)
	if err != nil {
		return err
	}
	fm, content, err := SplitFrontmatter(it.Body)
	if err != nil {
		return err
	}
	fm["enabled"] = enabled
	raw, err := yaml.Marshal(fm)
	if err != nil {
		return err
	}
	body := "---\n" + string(raw) + "---\n" + content
	if err := os.WriteFile(it.Path, []byte(body), 0o644); err != nil {
		return err
	}
	h.commit("enable skill " + name)
	return nil
}

func (h *Hub) Add(src string) error {
	src = strings.TrimSpace(src)
	if src == "" {
		return fmt.Errorf("path or git url is required")
	}
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "git@") {
		tmp, err := os.MkdirTemp("", "devhub-skill-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		cmd := exec.Command("git", "clone", "--depth", "1", src, tmp)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git clone: %s: %w", bytesTrim(out), err)
		}
		src = tmp
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		p := filepath.Join(src, "SKILL.md")
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("%s has no SKILL.md", src)
		}
		return h.ImportFile(p, filepath.Base(src))
	}
	return h.ImportFile(src, "")
}

func bytesTrim(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

func (h *Hub) NewSkill(name, scope string) (string, error) {
	dir := filepath.Join(h.Root, "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, "SKILL.md")
	body := fmt.Sprintf("---\nname: %s\ndescription: TODO\nscope: %s\ntargets: [cursor]\n---\n\n# %s\n\n", name, scope, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		return "", err
	}
	h.commit("add skill " + name)
	return p, nil
}

func (h *Hub) NewRule(name, scope string) (string, error) {
	p := filepath.Join(h.Root, "rules", name+".md")
	body := fmt.Sprintf("---\ndescription: TODO\nscope: %s\nglobs: \"**/*\"\nalwaysApply: false\ntargets: [cursor]\n---\n\n", scope)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		return "", err
	}
	h.commit("add rule " + name)
	return p, nil
}

func (h *Hub) ImportFile(src, name string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	m, _, err := ParseSkill(string(b))
	if err != nil {
		return err
	}
	if name == "" {
		name = m.Name
	}
	dir := filepath.Join(h.Root, "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), b, 0o644); err != nil {
		return err
	}
	h.commit("import skill " + name)
	return nil
}

type Adapter interface {
	Name() string
	Sync(h *Hub, items []Item, dry bool) ([]string, error)
	Unsync(h *Hub, manifest Manifest) error
	Doctor(h *Hub, items []Item, manifest Manifest) []Issue
}

type Issue struct {
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

func enabledAdapters(cfg *config.Config, home string) []Adapter {
	want := map[string]bool{}
	for _, t := range cfg.Skills.Targets {
		want[strings.ToLower(t)] = true
	}
	var out []Adapter
	if want["cursor"] {
		out = append(out, CursorAdapter{Home: home})
	}
	if want["claude"] {
		out = append(out, ClaudeAdapter{Home: home})
	}
	if want["codex"] {
		out = append(out, CodexAdapter{Home: home})
	}
	return out
}

func (h *Hub) manifestPath() string {
	return filepath.Join(h.Root, ".sync-manifest.json")
}

func (h *Hub) LoadManifest() Manifest {
	b, err := os.ReadFile(h.manifestPath())
	if err != nil {
		return Manifest{}
	}
	var m Manifest
	_ = json.Unmarshal(b, &m)
	return m
}

func (h *Hub) saveManifest(m Manifest) error {
	b, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(h.manifestPath(), b, 0o644)
}

func (h *Hub) Sync(dry bool) (Manifest, error) {
	items, err := h.List()
	if err != nil {
		return Manifest{}, err
	}
	var files []string
	for _, a := range h.Adapters {
		wrote, err := a.Sync(h, items, dry)
		if err != nil {
			return Manifest{}, err
		}
		files = append(files, wrote...)
	}
	m := Manifest{Files: unique(files)}
	if !dry {
		_ = h.saveManifest(m)
	}
	return m, nil
}

func (h *Hub) Unsync() error {
	m := h.LoadManifest()
	for _, a := range h.Adapters {
		_ = a.Unsync(h, m)
	}
	return h.saveManifest(Manifest{})
}

func (h *Hub) Doctor() []Issue {
	items, _ := h.List()
	m := h.LoadManifest()
	var issues []Issue
	for _, a := range h.Adapters {
		issues = append(issues, a.Doctor(h, items, m)...)
	}
	return issues
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func linkOrCopy(src, dst string, dry bool) error {
	if dry {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	_ = os.Remove(dst)
	if err := os.Symlink(src, dst); err != nil {
		b, err2 := os.ReadFile(src)
		if err2 != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	}
	return nil
}

func writeManaged(path, body string, dry bool) error {
	if dry {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

func removeIfManaged(path string, manifest Manifest) {
	ok := false
	for _, f := range manifest.Files {
		if f == path {
			ok = true
			break
		}
	}
	if !ok {
		return
	}
	_ = os.Remove(path)
}

func projectPath(cfg *config.Config, scope string) (string, string) {
	if strings.HasPrefix(scope, "project:") {
		name := strings.TrimPrefix(scope, "project:")
		p, err := cfg.ProjectPath(name)
		if err != nil {
			return name, ""
		}
		return name, p
	}
	return "", ""
}

func WaitDebounce(d time.Duration) { time.Sleep(d) }
