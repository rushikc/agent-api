package scan

import (
	"os"
	"path/filepath"

	"github.com/rushikesh/devhub/internal/config"
)

func DesktopRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Desktop")
}

func HomeRoot() string {
	home, _ := os.UserHomeDir()
	return home
}

func VolumeRoots() []string {
	entries, err := os.ReadDir("/Volumes")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || e.Type()&os.ModeSymlink != 0 {
			continue
		}
		out = append(out, filepath.Join("/Volumes", e.Name()))
	}
	return out
}

func ResolveRoots(cfg *config.Config, all bool, extra []string, includeVolumes bool) []string {
	var roots []string
	seen := map[string]struct{}{}
	add := func(p string) {
		if p == "" {
			return
		}
		expanded, err := config.ExpandPath(p)
		if err != nil {
			return
		}
		expanded = filepath.Clean(expanded)
		if _, ok := seen[expanded]; ok {
			return
		}
		seen[expanded] = struct{}{}
		roots = append(roots, expanded)
	}
	if all {
		add(HomeRoot())
	} else {
		add(DesktopRoot())
	}
	if includeVolumes {
		for _, v := range VolumeRoots() {
			add(v)
		}
	}
	if cfg != nil {
		for _, r := range cfg.Scan.Roots {
			add(r)
		}
	}
	for _, r := range extra {
		add(r)
	}
	return roots
}

func ExtraAllowRoots(cfg *config.Config, extra []string) []string {
	var out []string
	if cfg != nil {
		out = append(out, cfg.Scan.Roots...)
	}
	out = append(out, extra...)
	return out
}

func ResolveDepth(cfg *config.Config, all bool, flag int) int {
	if flag > 0 {
		return flag
	}
	if cfg == nil {
		if all {
			return 10
		}
		return 6
	}
	if all {
		return cfg.Scan.DepthAll
	}
	return cfg.Scan.DepthDesktop
}
