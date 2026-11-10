package profile

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

var (
	envPortRe    = regexp.MustCompile(`(?m)^PORT\s*=\s*(\d{2,5})`)
	flagPortRe   = regexp.MustCompile(`(?i)--port(?:=|\s+)(\d{2,5})|-p\s+(\d{2,5})`)
	listenRe     = regexp.MustCompile(`(?i)(?:\.listen|listen)\(\s*(\d{2,5})`)
	serverPortRe = regexp.MustCompile(`(?i)server\.port\s*[:=]\s*(\d{2,5})`)
	cfgPortRe    = regexp.MustCompile(`(?i)port\s*[:=]\s*(\d{2,5})`)
)

func DetectPort(dir string) int {
	var envFiles []string
	if matches, err := filepath.Glob(filepath.Join(dir, ".env*")); err == nil {
		envFiles = matches
	}
	configFiles := []string{
		filepath.Join(dir, "vite.config.ts"),
		filepath.Join(dir, "vite.config.js"),
		filepath.Join(dir, "vite.config.mjs"),
		filepath.Join(dir, "next.config.js"),
		filepath.Join(dir, "next.config.mjs"),
		filepath.Join(dir, "next.config.ts"),
		filepath.Join(dir, "server.js"),
		filepath.Join(dir, "index.js"),
		filepath.Join(dir, "app.js"),
		filepath.Join(dir, "app.properties"),
		filepath.Join(dir, "application.properties"),
	}
	if matches, err := filepath.Glob(filepath.Join(dir, "*.sh")); err == nil {
		configFiles = append(configFiles, matches...)
	}
	pkg := filepath.Join(dir, "package.json")
	for _, group := range [][]string{envFiles, configFiles, {pkg}} {
		for _, f := range group {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			if n := portFromBytes(b); n > 0 {
				return n
			}
		}
	}
	return 0
}

func portFromBytes(b []byte) int {
	s := string(b)
	for _, re := range []*regexp.Regexp{envPortRe, flagPortRe, listenRe, serverPortRe, cfgPortRe} {
		m := re.FindStringSubmatch(s)
		if n := firstPort(m); n > 0 {
			return n
		}
	}
	return 0
}

func firstPort(m []string) int {
	for i := 1; i < len(m); i++ {
		if m[i] == "" {
			continue
		}
		n, err := strconv.Atoi(m[i])
		if err == nil && n > 0 && n < 65536 {
			return n
		}
	}
	return 0
}
