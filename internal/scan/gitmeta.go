package scan

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func inspectRepo(dir string) Repo {
	r := Repo{
		Name: filepath.Base(dir),
		Path: dir,
	}
	gitDir, ok := resolveGitDir(dir)
	if !ok {
		r.Stack, r.HasDevhubYAML = detectStack(dir)
		return r
	}
	r.Branch = readHEAD(gitDir)
	r.Remote = readOrigin(gitDir)
	r.LastActivity = lastActivity(gitDir, r.Branch)
	r.Stack, r.HasDevhubYAML = detectStack(dir)
	return r
}

func resolveGitDir(repo string) (string, bool) {
	p := filepath.Join(repo, ".git")
	info, err := os.Lstat(p)
	if err != nil {
		return "", false
	}
	if info.IsDir() {
		return p, true
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(p)
		if err != nil {
			return "", false
		}
		st, err := os.Stat(target)
		if err != nil {
			return "", false
		}
		if st.IsDir() {
			return target, true
		}
		return parseGitDirFile(target, repo)
	}
	if info.Mode().IsRegular() {
		return parseGitDirFile(p, repo)
	}
	return "", false
}

func parseGitDirFile(path, repo string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(b))
	const prefix = "gitdir:"
	if !strings.HasPrefix(strings.ToLower(line), prefix) {
		return "", false
	}
	raw := strings.TrimSpace(line[len(prefix):])
	if raw == "" {
		return "", false
	}
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(repo, raw)
	}
	return filepath.Clean(raw), true
}

func isGitRepo(dir string, entries []os.DirEntry) bool {
	for _, e := range entries {
		if e.Name() == ".git" {
			return true
		}
	}
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}

func readHEAD(gitDir string) string {
	b, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(b))
	const refPrefix = "ref: "
	if strings.HasPrefix(line, refPrefix) {
		ref := strings.TrimSpace(line[len(refPrefix):])
		if strings.HasPrefix(ref, "refs/heads/") {
			return strings.TrimPrefix(ref, "refs/heads/")
		}
		return ref
	}
	if line == "" {
		return ""
	}
	if len(line) > 7 {
		return "detached@" + line[:7]
	}
	return "detached@" + line
}

func readOrigin(gitDir string) string {
	f, err := os.Open(filepath.Join(gitDir, "config"))
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	inOrigin := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inOrigin = strings.EqualFold(line, `[remote "origin"]`)
			continue
		}
		if !inOrigin {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(strings.ToLower(k)) == "url" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func lastActivity(gitDir, branch string) time.Time {
	candidates := []string{
		filepath.Join(gitDir, "logs", "HEAD"),
	}
	if branch != "" && !strings.HasPrefix(branch, "detached@") {
		candidates = append(candidates, filepath.Join(gitDir, "refs", "heads", branch))
	}
	candidates = append(candidates, gitDir)
	var best time.Time
	for _, p := range candidates {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if st.ModTime().After(best) {
			best = st.ModTime()
		}
	}
	return best
}
