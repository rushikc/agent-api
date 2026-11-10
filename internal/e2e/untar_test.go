package e2e

import (
	"archive/tar"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractTarNestedJUnit(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	files := map[string]string{
		"junit.xml":              `<?xml version="1.0"?><testsuite><testcase name="ok"/><testcase name="bad"><failure message="boom"/></testcase></testsuite>`,
		"nested/dir/results.xml": `<?xml version="1.0"?><testsuite><testcase name="skip"><skipped/></testcase></testsuite>`,
	}
	for name, body := range files {
		_ = tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(body)), Mode: 0644})
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.Close()
	dir := t.TempDir()
	if err := ExtractTar(&buf, dir); err != nil {
		t.Fatal(err)
	}
	sum, ok := SummarizeJUnitDir(dir)
	if !ok || sum.Passed != 1 || sum.Failed != 1 || sum.Skipped != 1 {
		t.Fatalf("%+v ok=%v", sum, ok)
	}
	if len(sum.Failures) != 1 || sum.Failures[0].Name != "bad" {
		t.Fatalf("%+v", sum.Failures)
	}
}

func TestExtractTarStripsTopLevelDir(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	body := `<?xml version="1.0"?><testsuite><testcase name="ok"/></testsuite>`
	_ = tw.WriteHeader(&tar.Header{Name: "junit.xml/", Mode: 0755, Typeflag: tar.TypeDir})
	_ = tw.WriteHeader(&tar.Header{Name: "junit.xml/junit.xml", Size: int64(len(body)), Mode: 0644})
	_, _ = tw.Write([]byte(body))
	_ = tw.Close()
	dir := t.TempDir()
	dest := filepath.Join(dir, "junit.xml")
	if err := ExtractTar(&buf, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Fatalf("%s", got)
	}
}

func TestExtractTarRejectsEscape(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	body := "secret"
	_ = tw.WriteHeader(&tar.Header{Name: "../evil.txt", Size: int64(len(body)), Mode: 0644})
	_, _ = tw.Write([]byte(body))
	_ = tw.WriteHeader(&tar.Header{Name: "/tmp/abs.txt", Size: int64(len(body)), Mode: 0644})
	_, _ = tw.Write([]byte(body))
	_ = tw.Close()
	dir := t.TempDir()
	if err := ExtractTar(&buf, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "evil.txt")); err == nil {
		if b, _ := os.ReadFile(filepath.Join(filepath.Dir(dir), "evil.txt")); strings.Contains(string(b), "secret") {
			t.Fatal("escaped")
		}
	}
}

func TestDemuxAndBuildLogs(t *testing.T) {
	var mux bytes.Buffer
	payload := []byte("hello from container\n")
	var hdr [8]byte
	hdr[0] = 1
	binary.BigEndian.PutUint32(hdr[4:], uint32(len(payload)))
	mux.Write(hdr[:])
	mux.Write(payload)
	var out bytes.Buffer
	if err := DemuxDockerLogs(&mux, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != string(payload) {
		t.Fatalf("%q", out.String())
	}
	in := strings.NewReader(`{"stream":"Step 1/2\n"}` + "\n" + `{"status":"Pulling","id":"abc"}` + "\n" + `{"error":"boom"}` + "\n")
	out.Reset()
	if err := DecodeBuildStream(in, &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "Step 1/2") || !strings.Contains(s, "abc Pulling") || !strings.Contains(s, "ERROR: boom") {
		t.Fatalf("%s", s)
	}
}
