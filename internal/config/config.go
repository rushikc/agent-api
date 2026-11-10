package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DefaultPrefix = "C-Space"
	DirName       = ".devhub"
	ConfigName    = "config.yaml"
	DBName        = "devhub.db"
	StatusName    = "status.json"
	TmuxConfName  = "tmux.conf"
	MonitorLock   = "monitor.lock"
	SocketName    = "devhub.sock"
	PidName       = "daemon.pid"
	TokenName     = "token"
	LogDirName    = "logs"
	ScanCacheName = "scan-cache.json"
)

type Keys struct {
	Mode string `yaml:"mode"` // option | prefix | both
}

type APIConfig struct {
	EnableTCP bool `yaml:"enable_tcp"`
	TCPPort   int  `yaml:"tcp_port"`
}

type MCPConfig struct {
	AllowMutations  bool `yaml:"allow_mutations"`
	AllowPortKill   bool `yaml:"allow_port_kill"`
	AllowProdViaMCP bool `yaml:"allow_prod_via_mcp"`
}

type SkillsConfig struct {
	Targets []string `yaml:"targets"`
}

type JanitorConfig struct {
	ProcessNames []string `yaml:"process_names"`
}

type GCPConfig struct {
	Projects                  []GCPProject `yaml:"projects"`
	ImpersonateServiceAccount string       `yaml:"impersonate_service_account,omitempty"`
	AllowProdViaMCP           bool         `yaml:"allow_prod_via_mcp"`
	RateLimitPerMin           int          `yaml:"rate_limit_per_min"`
}

type GCPProject struct {
	ID                        string   `yaml:"id"`
	Alias                     string   `yaml:"alias"`
	Allow                     []string `yaml:"allow"`
	RequireConfirm            bool     `yaml:"require_confirm"`
	ImpersonateServiceAccount string   `yaml:"impersonate_service_account,omitempty"`
	BQ                        BQPolicy `yaml:"bq"`
}

type BQPolicy struct {
	MaxBytesBilled string `yaml:"max_bytes_billed"`
	DryRunFirst    bool   `yaml:"dry_run_first"`
}

type E2EGlobalConfig struct {
	Concurrency int    `yaml:"concurrency"`
	Retention   string `yaml:"retention"`
}

type ScanConfig struct {
	Roots          []string `yaml:"roots"`
	Exclude        []string `yaml:"exclude"`
	DepthDesktop   int      `yaml:"depth_desktop"`
	DepthAll       int      `yaml:"depth_all"`
	IncludeVolumes bool     `yaml:"include_volumes"`
}

type AlertsConfig struct {
	QuietHours QuietHours  `yaml:"quiet_hours"`
	Slack      SlackConfig `yaml:"slack"`
	Rules      []AlertRule `yaml:"rules"`
}

type QuietHours struct {
	Start    string `yaml:"start"`
	End      string `yaml:"end"`
	Location string `yaml:"location"`
}

type SlackConfig struct {
	Webhook string `yaml:"webhook"`
}

type AlertRule struct {
	On      string    `yaml:"on"`
	When    AlertWhen `yaml:"when"`
	Notify  []string  `yaml:"notify"`
	Actions []string  `yaml:"actions"`
}

type AlertWhen struct {
	RSSMB  int     `yaml:"rss_mb"`
	CPUPct float64 `yaml:"cpu_pct"`
	For    string  `yaml:"for"`
}

type Config struct {
	Prefix                string              `yaml:"prefix"`
	Keys                  Keys                `yaml:"keys"`
	Projects              map[string]string   `yaml:"projects"`
	ProjectColors         map[string]string   `yaml:"project_colors"`
	DefaultShadowCommands map[string][]string `yaml:"default_shadow_commands"`
	API                   APIConfig           `yaml:"api"`
	MCP                   MCPConfig           `yaml:"mcp"`
	Skills                SkillsConfig        `yaml:"skills"`
	Janitor               JanitorConfig       `yaml:"janitor"`
	GCP                   GCPConfig           `yaml:"gcp"`
	E2E                   E2EGlobalConfig     `yaml:"e2e"`
	Alerts                AlertsConfig        `yaml:"alerts"`
	Scan                  ScanConfig          `yaml:"scan"`
	path                  string
	dir                   string
}

func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, DirName), nil
}

func Default() *Config {
	return &Config{
		Prefix:                DefaultPrefix,
		Keys:                  Keys{Mode: "both"},
		Projects:              map[string]string{},
		ProjectColors:         map[string]string{},
		DefaultShadowCommands: map[string][]string{},
		API:                   APIConfig{EnableTCP: false, TCPPort: 7420},
		MCP:                   MCPConfig{AllowMutations: true, AllowPortKill: false},
		Skills:                SkillsConfig{Targets: []string{"cursor"}},
		Janitor: JanitorConfig{ProcessNames: []string{
			"node", "npm", "pnpm", "bun", "go", "python", "python3", "vite", "next",
		}},
		GCP:  GCPConfig{RateLimitPerMin: 30},
		E2E:  E2EGlobalConfig{Concurrency: 2, Retention: "7d"},
		Scan: ScanConfig{DepthDesktop: 6, DepthAll: 10},
	}
}

func Load() (*Config, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	path := filepath.Join(dir, ConfigName)
	cfg := Default()
	cfg.dir = dir
	cfg.path = path

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if err := cfg.Save(); err != nil {
				return nil, err
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.normalize()
	cfg.dir = dir
	cfg.path = path
	return cfg, nil
}

func (c *Config) Save() error {
	if c.path == "" {
		dir, err := Dir()
		if err != nil {
			return err
		}
		c.dir = dir
		c.path = filepath.Join(dir, ConfigName)
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(c.path, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func (c *Config) Dir() string {
	if c.dir != "" {
		return c.dir
	}
	dir, _ := Dir()
	return dir
}

func (c *Config) WithDir(dir string) *Config {
	c.dir = dir
	c.path = filepath.Join(dir, ConfigName)
	return c
}

func (c *Config) DBPath() string {
	return filepath.Join(c.Dir(), DBName)
}

func (c *Config) StatusPath() string {
	return filepath.Join(c.Dir(), StatusName)
}

func (c *Config) TmuxConfPath() string {
	return filepath.Join(c.Dir(), TmuxConfName)
}

func (c *Config) MonitorLockPath() string {
	return filepath.Join(c.Dir(), MonitorLock)
}

func (c *Config) SocketPath() string {
	return filepath.Join(c.Dir(), SocketName)
}

func (c *Config) PidPath() string {
	return filepath.Join(c.Dir(), PidName)
}

func (c *Config) TokenPath() string {
	return filepath.Join(c.Dir(), TokenName)
}

func (c *Config) LogDir() string {
	return filepath.Join(c.Dir(), LogDirName)
}

func (c *Config) DaemonLogPath() string {
	return filepath.Join(c.LogDir(), "daemon.log")
}

func (c *Config) SkillsDir() string {
	return filepath.Join(c.Dir(), "skills")
}

func (c *Config) E2EDir() string {
	return filepath.Join(c.Dir(), "e2e")
}

func (c *Config) ScanCachePath() string {
	return filepath.Join(c.Dir(), ScanCacheName)
}

func (c *Config) AllowProdViaMCP() bool {
	return c.MCP.AllowProdViaMCP || c.GCP.AllowProdViaMCP
}

func MergeGCP(global GCPConfig, overlay []GCPProject) GCPConfig {
	out := global
	if len(overlay) == 0 {
		return out
	}
	byAlias := map[string]GCPProject{}
	for _, p := range global.Projects {
		key := p.Alias
		if key == "" {
			key = p.ID
		}
		byAlias[key] = p
	}
	for _, p := range overlay {
		key := p.Alias
		if key == "" {
			key = p.ID
		}
		byAlias[key] = p
	}
	out.Projects = out.Projects[:0]
	for _, p := range byAlias {
		out.Projects = append(out.Projects, p)
	}
	return out
}

func (c *Config) KeysMode() string {
	m := strings.ToLower(strings.TrimSpace(c.Keys.Mode))
	switch m {
	case "option", "prefix", "both":
		return m
	default:
		return "both"
	}
}

func (c *Config) ColorFor(project string) string {
	if c.ProjectColors != nil {
		if v := strings.TrimSpace(c.ProjectColors[project]); v != "" {
			return v
		}
	}
	return HashColor(project)
}

func CanonPath(p string) string {
	expanded, err := ExpandPath(p)
	if err != nil {
		return filepath.Clean(p)
	}
	expanded = filepath.Clean(expanded)
	if resolved, err := filepath.EvalSymlinks(expanded); err == nil {
		return filepath.Clean(resolved)
	}
	dir, suffix := expanded, ""
	for {
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Clean(filepath.Join(resolved, suffix))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		suffix = filepath.Join(filepath.Base(dir), suffix)
		dir = parent
	}
	return expanded
}

func (c *Config) Reload() error {
	if c.path == "" {
		return fmt.Errorf("config path is not set")
	}
	data, err := os.ReadFile(c.path)
	if err != nil {
		return err
	}
	next := Default()
	if err := yaml.Unmarshal(data, next); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	next.normalize()
	next.dir = c.dir
	next.path = c.path
	*c = *next
	return nil
}

func (c *Config) normalize() {
	if c.Prefix == "" {
		c.Prefix = DefaultPrefix
	}
	if c.Keys.Mode == "" {
		c.Keys.Mode = "both"
	}
	if c.Projects == nil {
		c.Projects = map[string]string{}
	}
	if c.ProjectColors == nil {
		c.ProjectColors = map[string]string{}
	}
	if c.DefaultShadowCommands == nil {
		c.DefaultShadowCommands = map[string][]string{}
	}
	if c.API.TCPPort == 0 {
		c.API.TCPPort = 7420
	}
	if len(c.Skills.Targets) == 0 {
		c.Skills.Targets = []string{"cursor"}
	}
	if len(c.Janitor.ProcessNames) == 0 {
		c.Janitor.ProcessNames = Default().Janitor.ProcessNames
	}
	if c.GCP.RateLimitPerMin <= 0 {
		c.GCP.RateLimitPerMin = 30
	}
	if c.E2E.Concurrency <= 0 {
		c.E2E.Concurrency = 2
	}
	if c.E2E.Retention == "" {
		c.E2E.Retention = "7d"
	}
	if c.Scan.DepthDesktop <= 0 {
		c.Scan.DepthDesktop = 6
	}
	if c.Scan.DepthAll <= 0 {
		c.Scan.DepthAll = 10
	}
	if c.Scan.Roots == nil {
		c.Scan.Roots = []string{}
	}
	if c.Scan.Exclude == nil {
		c.Scan.Exclude = []string{}
	}
}

func (c *Config) ProjectPath(name string) (string, error) {
	p, ok := c.Projects[name]
	if !ok {
		return "", fmt.Errorf("unknown project %q (add it with: devhub project add %s <path>)", name, name)
	}
	expanded, err := ExpandPath(p)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(expanded)
	if err != nil {
		return "", fmt.Errorf("project %q path %s: %w", name, expanded, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project %q path %s is not a directory", name, expanded)
	}
	return CanonPath(expanded), nil
}

func (c *Config) AddProject(name, path string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("project name is required")
	}
	expanded, err := ExpandPath(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(expanded)
	if err != nil {
		msg := fmt.Sprintf("path %s: %v", expanded, err)
		if hints := SuggestNearby(expanded); len(hints) > 0 {
			msg += "\nDid you mean " + strings.Join(hints, ", ") + "?"
		}
		return fmt.Errorf("%s", msg)
	}
	if !info.IsDir() {
		return fmt.Errorf("path %s is not a directory", expanded)
	}
	if c.Projects == nil {
		c.Projects = map[string]string{}
	}
	c.Projects[name] = CanonPath(expanded)
	return c.Save()
}

func (c *Config) RemoveProject(name string) error {
	if _, ok := c.Projects[name]; !ok {
		return fmt.Errorf("unknown project %q", name)
	}
	delete(c.Projects, name)
	return c.Save()
}

func (c *Config) UniqueProjectName(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "project"
	}
	if c.Projects == nil {
		return base
	}
	if _, ok := c.Projects[base]; !ok {
		return base
	}
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s-%d", base, i)
		if _, ok := c.Projects[cand]; !ok {
			return cand
		}
	}
}

func (c *Config) ProjectNameForPath(canon string) (string, bool) {
	if c == nil || c.Projects == nil {
		return "", false
	}
	want := CanonPath(canon)
	for name, p := range c.Projects {
		expanded, err := ExpandPath(p)
		if err != nil {
			continue
		}
		if CanonPath(expanded) == want {
			return name, true
		}
	}
	return "", false
}

func (c *Config) RenameProject(old, newName string) error {
	old = strings.TrimSpace(old)
	newName = strings.TrimSpace(newName)
	if old == "" || newName == "" {
		return fmt.Errorf("project name is required")
	}
	if _, ok := c.Projects[old]; !ok {
		return fmt.Errorf("unknown project %q", old)
	}
	if old == newName {
		return nil
	}
	if _, ok := c.Projects[newName]; ok {
		return fmt.Errorf("project %q already exists", newName)
	}
	c.Projects[newName] = c.Projects[old]
	delete(c.Projects, old)
	if c.ProjectColors != nil {
		if col, ok := c.ProjectColors[old]; ok {
			c.ProjectColors[newName] = col
			delete(c.ProjectColors, old)
		}
	}
	return c.Save()
}

func (c *Config) MoveProject(name, path string) error {
	if _, ok := c.Projects[name]; !ok {
		return fmt.Errorf("unknown project %q", name)
	}
	expanded, err := ExpandPath(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(expanded)
	if err != nil {
		return fmt.Errorf("path %s: %w", expanded, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path %s is not a directory", expanded)
	}
	c.Projects[name] = CanonPath(expanded)
	return c.Save()
}

func SuggestNearby(path string) []string {
	path = filepath.Clean(path)
	parent := filepath.Dir(path)
	base := filepath.Base(path)
	var hints []string
	seen := map[string]bool{}
	add := func(p string) {
		p = filepath.Clean(p)
		if seen[p] {
			return
		}
		st, err := os.Stat(p)
		if err != nil || !st.IsDir() {
			return
		}
		seen[p] = true
		hints = append(hints, p)
	}
	entries, err := os.ReadDir(parent)
	if err == nil {
		lower := strings.ToLower(base)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			n := e.Name()
			if strings.EqualFold(n, base) || strings.Contains(strings.ToLower(n), lower) || strings.Contains(lower, strings.ToLower(n)) {
				add(filepath.Join(parent, n))
			}
		}
	}
	gp := filepath.Dir(parent)
	gents, err := os.ReadDir(gp)
	if err == nil {
		wantParent := strings.ToLower(filepath.Base(parent))
		for _, e := range gents {
			if !e.IsDir() {
				continue
			}
			n := e.Name()
			if strings.EqualFold(n, wantParent) || strings.EqualFold(n, wantParent+"s") || strings.EqualFold(n+"s", wantParent) {
				add(filepath.Join(gp, n, base))
				ents, _ := os.ReadDir(filepath.Join(gp, n))
				for _, c := range ents {
					if c.IsDir() && strings.EqualFold(c.Name(), base) {
						add(filepath.Join(gp, n, c.Name()))
					}
				}
			}
		}
	}
	repl := strings.NewReplacer("/project/", "/Projects/", "/Project/", "/Projects/")
	add(repl.Replace(path))
	if len(hints) > 8 {
		hints = hints[:8]
	}
	return hints
}

func ExpandPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("path is required")
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			return home, nil
		}
		p = filepath.Join(home, p[2:])
	}
	return filepath.Abs(p)
}
