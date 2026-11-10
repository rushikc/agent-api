package e2e

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"io"
	"strings"
)

func DecodeBuildStream(r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			if _, e := io.WriteString(w, line+"\n"); e != nil {
				return e
			}
			continue
		}
		if errMsg, _ := m["error"].(string); errMsg != "" {
			if _, e := io.WriteString(w, "ERROR: "+errMsg+"\n"); e != nil {
				return e
			}
			continue
		}
		if s, _ := m["stream"].(string); s != "" {
			if _, e := io.WriteString(w, s); e != nil {
				return e
			}
			if !strings.HasSuffix(s, "\n") {
				_, _ = io.WriteString(w, "\n")
			}
			continue
		}
		if s, _ := m["status"].(string); s != "" {
			id, _ := m["id"].(string)
			if id != "" {
				s = id + " " + s
			}
			if _, e := io.WriteString(w, s+"\n"); e != nil {
				return e
			}
		}
	}
	return sc.Err()
}

func DemuxDockerLogs(r io.Reader, w io.Writer) error {
	hdr := make([]byte, 8)
	for {
		n, err := io.ReadFull(r, hdr)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			if n > 0 {
				_, _ = w.Write(hdr[:n])
			}
			return nil
		}
		if err != nil {
			return err
		}
		if hdr[0] != 1 && hdr[0] != 2 {
			if _, err := w.Write(hdr); err != nil {
				return err
			}
			_, err = io.Copy(w, r)
			return err
		}
		size := binary.BigEndian.Uint32(hdr[4:8])
		if size == 0 {
			continue
		}
		if _, err := io.CopyN(w, r, int64(size)); err != nil {
			return err
		}
	}
}

type prefixWriter struct {
	w      io.Writer
	prefix string
	need   bool
}

func newPrefixWriter(w io.Writer, prefix string) *prefixWriter {
	return &prefixWriter{w: w, prefix: prefix, need: true}
}

func (p *prefixWriter) Write(b []byte) (int, error) {
	n := 0
	for len(b) > 0 {
		if p.need {
			if _, err := io.WriteString(p.w, p.prefix); err != nil {
				return n, err
			}
			p.need = false
		}
		i := 0
		for i < len(b) && b[i] != '\n' {
			i++
		}
		if i < len(b) {
			wn, err := p.w.Write(b[:i+1])
			n += wn
			if err != nil {
				return n, err
			}
			p.need = true
			b = b[i+1:]
			continue
		}
		wn, err := p.w.Write(b)
		n += wn
		return n, err
	}
	return n, nil
}
