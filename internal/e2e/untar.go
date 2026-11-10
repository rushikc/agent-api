package e2e

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxExtractBytes = 512 << 20

type tarEnt struct {
	hdr  *tar.Header
	body []byte
}

func ExtractTar(r io.Reader, dest string) error {
	root, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		return err
	}
	tr := tar.NewReader(r)
	var ents []tarEnt
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			continue
		}
		var body []byte
		if hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeRegA {
			if total+hdr.Size > maxExtractBytes {
				return fmt.Errorf("artifact extract exceeds %d bytes", maxExtractBytes)
			}
			body, err = io.ReadAll(io.LimitReader(tr, hdr.Size))
			if err != nil {
				return err
			}
			total += int64(len(body))
		}
		h := *hdr
		ents = append(ents, tarEnt{hdr: &h, body: body})
	}
	base := filepath.Base(root)
	strip := shouldStrip(ents, base)
	for _, e := range ents {
		name := filepath.Clean(e.hdr.Name)
		if strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
			continue
		}
		if strip {
			name = stripName(name, base)
		}
		var target string
		if name == "" || name == "." || name == base {
			target = root
		} else {
			target = filepath.Join(root, name)
		}
		rel, err := filepath.Rel(filepath.Dir(root), target)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		if target != root {
			if rel2, err := filepath.Rel(root, target); err != nil || strings.HasPrefix(rel2, "..") {
				continue
			}
		}
		switch e.hdr.Typeflag {
		case tar.TypeDir:
			if target == root {
				continue
			}
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(target, e.body, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

func shouldStrip(ents []tarEnt, base string) bool {
	if base == "" || base == "." {
		return false
	}
	saw := false
	for _, e := range ents {
		name := filepath.ToSlash(filepath.Clean(e.hdr.Name))
		if name == "." {
			continue
		}
		first := name
		if i := strings.IndexByte(name, '/'); i >= 0 {
			first = name[:i]
		}
		if first != base {
			return false
		}
		saw = true
	}
	return saw
}

func stripName(name, base string) string {
	name = filepath.ToSlash(filepath.Clean(name))
	if name == base {
		return ""
	}
	pref := base + "/"
	if strings.HasPrefix(name, pref) {
		return strings.TrimPrefix(name, pref)
	}
	return name
}
