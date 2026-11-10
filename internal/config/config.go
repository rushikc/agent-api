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
)

type Keys struct {
	Mode string `yaml:"mode"` // option | prefix | both
}

type APIConfig struct {
	EnableTCP bool `yaml:"enable_tcp"`
	TCPPort   int  `yaml:"tcp_port"`
}

type MCPConfig struct {
	AllowMutations bool `yaml:"allow_mutations"`
	AllowPortKill  bool `yaml:"allow_port_kill"`
}

type SkillsConfig struct {
	Targets []string `yaml:"targets"`
}

type JanitorConfig struct {
	ProcessNames []string `yaml:"process_names"`
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
	if cfg.Prefix == "" {
		cfg.Prefix = DefaultPrefix
	}
	if cfg.Keys.Mode == "" {
		cfg.Keys.Mode = "both"
	}
	if cfg.Projects == nil {
		cfg.Projects = map[string]string{}
	}
	if cfg.ProjectColors == nil {
		cfg.ProjectColors = map[string]string{}
	}
	if cfg.DefaultShadowCommands == nil {
		cfg.DefaultShadowCommands = map[string][]string{}
	}
	if cfg.API.TCPPort == 0 {
		cfg.API.TCPPort = 7420
	}
	if len(cfg.Skills.Targets) == 0 {
		cfg.Skills.Targets = []string{"cursor"}
	}
	if len(cfg.Janitor.ProcessNames) == 0 {
		cfg.Janitor.ProcessNames = Default().Janitor.ProcessNames
	}
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
	return expanded, nil
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
		return fmt.Errorf("path %s: %w", expanded, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path %s is not a directory", expanded)
	}
	if c.Projects == nil {
		c.Projects = map[string]string{}
	}
	c.Projects[name] = expanded
	return c.Save()
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
