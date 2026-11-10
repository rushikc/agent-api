package profile

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

var (
	flagPortRe = regexp.MustCompile(`(?i)(?:--port|-p)\s*=?\s*(\d{2,5})`)
	envPortRe  = regexp.MustCompile(`(?m)^PORT\s*=\s*(\d{2,5})`)
	cfgPortRe  = regexp.MustCompile(`(?i)port\s*[:=]\s*(\d{2,5})`)
)

func DetectPort(dir string) int {
	files := []string{
		filepath.Join(dir, "package.json"),
		filepath.Join(dir, ".env"),
		filepath.Join(dir, ".env.local"),
		filepath.Join(dir, "vite.config.ts"),
		filepath.Join(dir, "vite.config.js"),
		filepath.Join(dir, "vite.config.mjs"),
		filepath.Join(dir, "next.config.js"),
		filepath.Join(dir, "next.config.mjs"),
		filepath.Join(dir, "next.config.ts"),
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if n := portFromBytes(b); n > 0 {
			return n
		}
	}
	return 0
}

func portFromBytes(b []byte) int {
	s := string(b)
	for _, re := range []*regexp.Regexp{flagPortRe, envPortRe, cfgPortRe} {
		m := re.FindStringSubmatch(s)
		if len(m) == 2 {
			n, err := strconv.Atoi(m[1])
			if err == nil && n > 0 && n < 65536 {
				return n
			}
		}
	}
	return 0
}
