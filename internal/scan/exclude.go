package scan

import (
	"os"
	"path/filepath"
	"strings"
)

var skipNames = map[string]struct{}{
	"node_modules": {},
	".Trash":       {},
	"Library":      {},
	".cache":       {},
	".npm":         {},
	".pnpm-store":  {},
	".cargo":       {},
	".gradle":      {},
	".m2":          {},
	"venv":         {},
	".venv":        {},
	"__pycache__":  {},
	"vendor":       {},
	"Pods":         {},
	"DerivedData":  {},
	".docker":      {},
	".orbstack":    {},
	"Applications": {},
}

var skipPrefixes = []string{
	"/System",
	"/private",
	"/usr",
	"/bin",
	"/sbin",
	"/opt/homebrew",
}

var homeToolDirs = []string{
	".nvm",
	".cursor",
	".codex",
	".claude",
	".vscode",
	".rustup",
	".cargo",
	".pyenv",
	".rbenv",
	".sdkman",
	".gradle",
	".m2",
	".npm",
	".pnpm-store",
	".bun",
	".deno",
	".local/share",
	".devhub",
}

type skipOpts struct {
	exclude       []string
	home          string
	includeHidden bool
	allow         []string
}

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

func skipPath(path string, opts skipOpts) bool {
	clean := filepath.Clean(path)
	base := filepath.Base(clean)
	home := opts.home
	if home == "" {
		home = homeDir()
	}
	home = filepath.Clean(home)

	if _, ok := skipNames[base]; ok {
		return true
	}
	if strings.HasSuffix(clean, string(filepath.Separator)+"go"+string(filepath.Separator)+"pkg") ||
		strings.HasSuffix(clean, "/go/pkg") {
		return true
	}
	for _, p := range skipPrefixes {
		if clean == p || strings.HasPrefix(clean, p+"/") {
			return true
		}
	}
	if home != "" {
		lib := filepath.Join(home, "Library")
		if clean == lib || strings.HasPrefix(clean, lib+string(filepath.Separator)) {
			return true
		}
		cloud := filepath.Join(home, "Library", "Mobile Documents")
		if clean == cloud || strings.HasPrefix(clean, cloud+string(filepath.Separator)) {
			return true
		}
		cs := filepath.Join(home, "Library", "CloudStorage")
		if clean == cs || strings.HasPrefix(clean, cs+string(filepath.Separator)) {
			return true
		}
		if !opts.includeHidden && !isAllowed(clean, opts.allow) {
			if filepath.Dir(clean) == home && strings.HasPrefix(base, ".") {
				return true
			}
			for _, t := range homeToolDirs {
				p := filepath.Join(home, t)
				if clean == p || strings.HasPrefix(clean, p+string(filepath.Separator)) {
					return true
				}
			}
		}
	}
	for _, g := range opts.exclude {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		if matchGlob(g, clean) || matchGlob(g, base) {
			return true
		}
	}
	return false
}

func isAllowed(path string, roots []string) bool {
	home := homeDir()
	for _, r := range roots {
		r = filepath.Clean(r)
		if r == "" || r == "." || (home != "" && r == filepath.Clean(home)) {
			continue
		}
		if path == r || strings.HasPrefix(path, r+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func matchGlob(pat, name string) bool {
	ok, err := filepath.Match(pat, name)
	if err == nil && ok {
		return true
	}
	ok, err = filepath.Match(pat, filepath.Base(name))
	if err == nil && ok {
		return true
	}
	if strings.Contains(pat, "**") {
		rest := strings.TrimPrefix(strings.ReplaceAll(pat, "**", "*"), "*/")
		ok, err = filepath.Match(rest, filepath.Base(name))
		return err == nil && ok
	}
	if strings.Contains(name, strings.Trim(pat, "*")) && strings.Contains(pat, "*") {
		ok, err = filepath.Match(pat, filepath.Base(name))
		return err == nil && ok
	}
	return false
}

func isPrivacyDir(path, home string) bool {
	if home == "" {
		home = homeDir()
	}
	if home == "" {
		return false
	}
	clean := filepath.Clean(path)
	home = filepath.Clean(home)
	for _, name := range []string{"Desktop", "Documents", "Downloads"} {
		p := filepath.Join(home, name)
		if clean == p || strings.HasPrefix(clean, p+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
