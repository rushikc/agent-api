package scan

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCacheFreshness(t *testing.T) {
	c := &Cache{
		ScannedAt: time.Now().Add(-time.Hour),
		Roots:     []string{"/a"},
		Depth:     6,
	}
	if !Fresh(c, []string{"/a"}, 6, false, false, DefaultMaxAge) {
		t.Fatal("hour-old cache should be fresh")
	}
	c.ScannedAt = time.Now().Add(-25 * time.Hour)
	if Fresh(c, []string{"/a"}, 6, false, false, DefaultMaxAge) {
		t.Fatal("stale")
	}
	c.ScannedAt = time.Now()
	if Fresh(c, []string{"/b"}, 6, false, false, DefaultMaxAge) {
		t.Fatal("roots mismatch")
	}
	if Fresh(c, []string{"/a"}, 6, false, false, 0) && !Fresh(c, []string{"/a"}, 6, true, false, DefaultMaxAge) {
		// nested mismatch
	}
	if Fresh(c, []string{"/a"}, 6, true, false, DefaultMaxAge) {
		t.Fatal("nested mismatch")
	}
	path := filepath.Join(t.TempDir(), "scan-cache.json")
	c.ScannedAt = time.Now()
	c.Roots = []string{"/a"}
	if err := SaveCache(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCache(path)
	if err != nil || got.Depth != 6 {
		t.Fatalf("%+v %v", got, err)
	}
	if Age(got) < 0 {
		t.Fatal("age")
	}
}
