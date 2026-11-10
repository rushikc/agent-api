package profile

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/janitor"
	"github.com/rushikesh/devhub/internal/jsonx"
	"github.com/rushikesh/devhub/internal/registry"
	"gopkg.in/yaml.v3"
)

type Profile struct {
	Name    string            `yaml:"name" json:"name"`
	Env     map[string]string `yaml:"env" json:"env"`
	EnvFile string            `yaml:"env_file" json:"env_file,omitempty"`
	Windows []Window          `yaml:"windows" json:"windows"`
	Shadows []Shadow          `yaml:"shadows" json:"shadows"`
	Skills  []string          `yaml:"skills" json:"skills"`
	GCP     GCPOverlay        `yaml:"gcp" json:"gcp,omitempty"`
	E2E     E2EConfig         `yaml:"e2e" json:"e2e,omitempty"`
	Path    string            `yaml:"-" json:"path"`
}

type GCPOverlay struct {
	Projects []config.GCPProject `yaml:"projects" json:"projects,omitempty"`
}

type E2EConfig struct {
	Image        string            `yaml:"image" json:"image"`
	Dockerfile   string            `yaml:"dockerfile" json:"dockerfile"`
	CacheVolumes []string          `yaml:"cache_volumes" json:"cache_volumes,omitempty"`
	Command      string            `yaml:"command" json:"command"`
	Services     []E2EService      `yaml:"services" json:"services,omitempty"`
	Env          map[string]string `yaml:"env" json:"env,omitempty"`
	Secrets      []string          `yaml:"secrets" json:"secrets,omitempty"`
	Artifacts    []string          `yaml:"artifacts" json:"artifacts,omitempty"`
	Timeout      string            `yaml:"timeout" json:"timeout,omitempty"`
	Resources    E2EResources      `yaml:"resources" json:"resources,omitempty"`
}

type E2EService struct {
	Name  string            `yaml:"name" json:"name"`
	Image string            `yaml:"image" json:"image"`
	Env   map[string]string `yaml:"env" json:"env,omitempty"`
	Ready E2EReady          `yaml:"ready" json:"ready,omitempty"`
}

type E2EReady struct {
	TCP int `yaml:"tcp" json:"tcp,omitempty"`
}

type E2EResources struct {
	CPUs   float64 `yaml:"cpus" json:"cpus,omitempty"`
	Memory string  `yaml:"memory" json:"memory,omitempty"`
}

type Window struct {
	Name    string `yaml:"name" json:"name"`
	Cwd     string `yaml:"cwd" json:"cwd"`
	Command string `yaml:"command" json:"command"`
}

type Shadow struct {
	Name      string   `yaml:"name" json:"name"`
	Command   string   `yaml:"command" json:"command"`
	Argv      []string `yaml:"argv" json:"argv,omitempty"`
	Cwd       string   `yaml:"cwd" json:"cwd"`
	Port      int      `yaml:"port" json:"port"`
	Ready     Ready    `yaml:"ready" json:"ready"`
	Restart   string   `yaml:"restart" json:"restart"` // no | on-failure | always
	DependsOn []string `yaml:"depends_on" json:"depends_on"`
}

type Ready struct {
	HTTP    string        `yaml:"http" json:"http"`
	Log     string        `yaml:"log" json:"log"`
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

func (r Ready) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		HTTP    string  `json:"http"`
		Log     string  `json:"log"`
		Timeout float64 `json:"timeout"`
	}{HTTP: r.HTTP, Log: r.Log, Timeout: jsonx.DurationMS(r.Timeout)})
}

func Load(dir string) (*Profile, error) {
	p := filepath.Join(dir, "devhub.yaml")
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var pr Profile
	if err := yaml.Unmarshal(b, &pr); err != nil {
		return nil, err
	}
	pr.Path = dir
	if pr.Name == "" {
		pr.Name = filepath.Base(dir)
	}
	return &pr, nil
}

func Validate(pr *Profile) error {
	if pr.Name == "" {
		return fmt.Errorf("name is required")
	}
	_, err := OrderShadows(pr.Shadows)
	return err
}

func OrderShadows(shadows []Shadow) ([]Shadow, error) {
	byName := map[string]Shadow{}
	for _, s := range shadows {
		if s.Name == "" {
			return nil, fmt.Errorf("shadow missing name")
		}
		byName[s.Name] = s
	}
	indeg := map[string]int{}
	edges := map[string][]string{}
	for _, s := range shadows {
		if _, ok := indeg[s.Name]; !ok {
			indeg[s.Name] = 0
		}
		for _, d := range s.DependsOn {
			if _, ok := byName[d]; !ok {
				return nil, fmt.Errorf("shadow %s depends on unknown %s", s.Name, d)
			}
			edges[d] = append(edges[d], s.Name)
			indeg[s.Name]++
		}
	}
	var q []string
	for n, d := range indeg {
		if d == 0 {
			q = append(q, n)
		}
	}
	var out []Shadow
	for len(q) > 0 {
		n := q[0]
		q = q[1:]
		out = append(out, byName[n])
		for _, m := range edges[n] {
			indeg[m]--
			if indeg[m] == 0 {
				q = append(q, m)
			}
		}
	}
	if len(out) != len(shadows) {
		return nil, fmt.Errorf("dependency cycle in shadows")
	}
	return out, nil
}

func absCwd(root, rel string) string {
	if rel == "" || rel == "." {
		return root
	}
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(root, rel)
}

func loadEnvFile(root, name string) map[string]string {
	out := map[string]string{}
	if name == "" {
		return out
	}
	b, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return out
}

func mergedEnv(pr *Profile) map[string]string {
	env := loadEnvFile(pr.Path, pr.EnvFile)
	for k, v := range pr.Env {
		env[k] = v
	}
	return env
}

func EnvForProject(cfg *config.Config, name string) map[string]string {
	pr, err := Find(cfg, name)
	if err != nil {
		return nil
	}
	return mergedEnv(pr)
}

type UpResult struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Port   int    `json:"port,omitempty"`
	URL    string `json:"url,omitempty"`
	Status string `json:"status"`
}

func Up(svc *core.Service, pr *Profile, autoFree bool) ([]UpResult, error) {
	if err := Validate(pr); err != nil {
		return nil, err
	}
	if _, ok := svc.Cfg.Projects[pr.Name]; !ok {
		if err := svc.Cfg.AddProject(pr.Name, pr.Path); err != nil {
			return nil, err
		}
	}
	env := mergedEnv(pr)
	var results []UpResult
	sessions, _ := svc.DB.List(false)
	for _, w := range pr.Windows {
		cwd := absCwd(pr.Path, w.Cwd)
		if _, err := svc.NewActiveIn(pr.Name, w.Name, cwd, env); err != nil {
			results = append(results, UpResult{Name: w.Name, Kind: "active", Status: err.Error()})
			continue
		}
		results = append(results, UpResult{Name: w.Name, Kind: "active", Status: "ok"})
	}
	ordered, err := OrderShadows(pr.Shadows)
	if err != nil {
		return results, err
	}
	for _, sh := range ordered {
		cwd := absCwd(pr.Path, sh.Cwd)
		if sh.Port > 0 {
			if p, taken := portTakenFn(sh.Port, sessions, nil); taken {
				svc.Emit("port.conflict", fmt.Sprintf("%s:%d", pr.Name, sh.Port), pr.Name, map[string]any{"port": sh.Port, "pid": p.PID, "owner": p.Owner})
				if autoFree {
					if !ShouldFreePort(p, pr.Name, sessions) {
						return results, fmt.Errorf("port %d in use by %s (pid %d); not killing a foreign process", sh.Port, p.Owner, p.PID)
					}
					_ = killTreeFn(p.PID)
				} else {
					return results, fmt.Errorf("port %d in use by %s (pid %d); pass -y to free", sh.Port, p.Owner, p.PID)
				}
			}
		}
		cmd := core.ResolveArgv(sh.Argv, sh.Command)
		sess, err := svc.NewShadowIn(pr.Name, sh.Name, cwd, cmd, env, sh.Port, sh.Restart)
		if err != nil {
			results = append(results, UpResult{Name: sh.Name, Kind: "shadow", Port: sh.Port, Status: err.Error()})
			continue
		}
		st := "ok"
		if err := waitReady(svc, sess, sh); err != nil {
			st = err.Error()
		}
		url := ""
		if sh.Ready.HTTP != "" {
			url = sh.Ready.HTTP
		} else if sh.Port > 0 {
			url = fmt.Sprintf("http://localhost:%d", sh.Port)
		}
		results = append(results, UpResult{Name: sh.Name, Kind: "shadow", Port: sh.Port, URL: url, Status: st})
		sessions, _ = svc.DB.List(false)
	}
	return results, nil
}

func waitReady(svc *core.Service, sess registry.Session, sh Shadow) error {
	timeout := sh.Ready.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if sh.Ready.HTTP != "" {
			resp, err := http.Get(sh.Ready.HTTP)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode < 500 {
					return nil
				}
			}
		} else if sh.Ready.Log != "" {
			out, err := svc.Peek(sess.Project + "/" + sess.Name)
			if err == nil && strings.Contains(out, sh.Ready.Log) {
				return nil
			}
		} else {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("not ready after %s", timeout)
}

var (
	portTakenFn = janitor.PortTaken
	killTreeFn  = janitor.KillTree
)

func ShouldFreePort(p janitor.Port, project string, sessions []registry.Session) bool {
	if p.Project == project || strings.HasPrefix(p.Owner, project+"/") {
		return true
	}
	for _, s := range sessions {
		if s.Project != project {
			continue
		}
		if s.ID == p.SessionID && s.ID != 0 {
			return true
		}
		if s.PanePID == int(p.PID) && s.Status != registry.StatusRunning {
			return true
		}
	}
	return false
}

func Down(svc *core.Service, project string, all bool) error {
	rows, err := svc.DB.List(true)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.Project != project {
			continue
		}
		if !all && r.Kind != registry.KindShadow {
			continue
		}
		if r.PanePID > 0 {
			_ = killTreeFn(int32(r.PanePID))
		}
		_ = svc.Stop(fmt.Sprintf("%s/%s", r.Project, r.Name))
		if r.Port > 0 {
			_ = janitor.FreePort(r.Port, rows, nil)
		}
	}
	return nil
}

func Init(dir string, cfg *config.Config) (*Profile, error) {
	name := filepath.Base(dir)
	if cfg != nil {
		want := config.CanonPath(dir)
		for n, p := range cfg.Projects {
			abs, err := config.ExpandPath(p)
			if err != nil {
				continue
			}
			if config.CanonPath(abs) == want {
				name = n
				break
			}
		}
	}
	pr := &Profile{Name: name, Path: dir, Env: map[string]string{}, Windows: []Window{{Name: "v1", Cwd: "."}}}
	if b, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		_ = json.Unmarshal(b, &pkg)
		if _, ok := pkg.Scripts["dev"]; ok {
			port := DetectPort(dir)
			if port == 0 {
				port = 3000
			}
			pr.Shadows = append(pr.Shadows, Shadow{Name: "web", Command: "npm run dev", Cwd: ".", Port: port, Restart: "on-failure"})
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		pr.Windows = append(pr.Windows, Window{Name: "go", Cwd: "."})
	}
	if _, err := os.Stat(filepath.Join(dir, "docker-compose.yml")); err == nil {
		pr.Shadows = append(pr.Shadows, Shadow{Name: "compose", Command: "docker compose up", Cwd: ".", Restart: "on-failure"})
	}
	if _, err := os.Stat(filepath.Join(dir, "Makefile")); err == nil && len(pr.Shadows) == 0 {
		pr.Windows = append(pr.Windows, Window{Name: "make", Cwd: "."})
	}
	b, err := yaml.Marshal(pr)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "devhub.yaml"), b, 0o644); err != nil {
		return nil, err
	}
	return pr, nil
}

func Find(cfg *config.Config, name string) (*Profile, error) {
	if name == "" {
		wd, _ := os.Getwd()
		if p, err := Load(wd); err == nil {
			return p, nil
		}
		return nil, fmt.Errorf("no devhub.yaml in %s (pass a project name or run from a repo)", wd)
	}
	path, err := cfg.ProjectPath(name)
	if err != nil {
		return nil, err
	}
	return Load(path)
}

func List(cfg *config.Config) []Profile {
	var out []Profile
	for n, p := range cfg.Projects {
		abs, err := config.ExpandPath(p)
		if err != nil {
			continue
		}
		pr, err := Load(abs)
		if err != nil {
			out = append(out, Profile{Name: n, Path: abs})
			continue
		}
		out = append(out, *pr)
	}
	return out
}
