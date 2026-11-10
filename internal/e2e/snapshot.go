package e2e

import (
	"archive/tar"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type RepoSnap struct {
	SHA   string
	Dirty bool
	Tar   []byte
}

func SnapshotRepo(dir string) (RepoSnap, error) {
	shaOut, err := git(dir, "rev-parse", "HEAD")
	sha := strings.TrimSpace(string(shaOut))
	if err != nil {
		sha = "unknown"
	}
	diff, _ := git(dir, "diff", "HEAD")
	dirty := len(bytes.TrimSpace(diff)) > 0
	list, err := git(dir, "ls-files", "-z")
	if err != nil {
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(dir, path)
			b, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			_ = tw.WriteHeader(&tar.Header{Name: rel, Mode: int64(info.Mode().Perm()), Size: int64(len(b))})
			_, _ = tw.Write(b)
			return nil
		})
		_ = tw.Close()
		return RepoSnap{SHA: sha, Dirty: true, Tar: buf.Bytes()}, nil
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, name := range strings.Split(string(list), "\x00") {
		if name == "" {
			continue
		}
		p := filepath.Join(dir, name)
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: int64(st.Mode().Perm()), Size: int64(len(b))}); err != nil {
			_ = tw.Close()
			return RepoSnap{}, err
		}
		if _, err := tw.Write(b); err != nil {
			_ = tw.Close()
			return RepoSnap{}, err
		}
	}
	if dirty {
		_ = tw.WriteHeader(&tar.Header{Name: ".devhub-diff.patch", Mode: 0644, Size: int64(len(diff))})
		_, _ = tw.Write(diff)
	}
	if err := tw.Close(); err != nil {
		return RepoSnap{}, err
	}
	return RepoSnap{SHA: sha, Dirty: dirty, Tar: buf.Bytes()}, nil
}

func git(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Output()
}

func ImageTag(image, dockerfilePath string, lockfiles []string) (string, error) {
	h := shaFiles(append([]string{dockerfilePath}, lockfiles...))
	if h == "" {
		h = "dev000000000"
	}
	if len(h) > 12 {
		h = h[:12]
	}
	if image == "" {
		image = "devhub/e2e"
	}
	return image + ":" + h, nil
}

func shaFiles(paths []string) string {
	var b []byte
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		b = append(b, raw...)
	}
	if len(b) == 0 {
		return ""
	}
	sum := sha256sum(b)
	return sum
}
