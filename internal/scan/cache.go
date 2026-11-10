package scan

import (
	"encoding/json"
	"os"
	"time"
)

const DefaultMaxAge = 24 * time.Hour

func LoadCache(path string) (*Cache, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Cache
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func SaveCache(path string, c *Cache) error {
	if c == nil {
		return nil
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func CacheFromResult(res *Result) *Cache {
	if res == nil {
		return nil
	}
	return &Cache{
		ScannedAt:      time.Now(),
		Roots:          append([]string(nil), res.Roots...),
		Depth:          res.Depth,
		Nested:         res.Nested,
		IncludeVolumes: res.IncludeVolumes,
		Dirs:           res.Dirs,
		Denied:         res.Denied,
		PrivacyDenied:  res.PrivacyDenied,
		Repos:          append([]Repo(nil), res.Repos...),
	}
}

func ResultFromCache(c *Cache) *Result {
	if c == nil {
		return nil
	}
	return &Result{
		Repos:          append([]Repo(nil), c.Repos...),
		Dirs:           c.Dirs,
		Denied:         c.Denied,
		PrivacyDenied:  c.PrivacyDenied,
		Roots:          append([]string(nil), c.Roots...),
		Depth:          c.Depth,
		Nested:         c.Nested,
		IncludeVolumes: c.IncludeVolumes,
	}
}

func Fresh(c *Cache, roots []string, depth int, nested, volumes bool, maxAge time.Duration) bool {
	if c == nil {
		return false
	}
	if maxAge <= 0 {
		maxAge = DefaultMaxAge
	}
	if time.Since(c.ScannedAt) > maxAge {
		return false
	}
	if c.Depth != depth || c.Nested != nested || c.IncludeVolumes != volumes {
		return false
	}
	if len(c.Roots) != len(roots) {
		return false
	}
	for i := range roots {
		if c.Roots[i] != roots[i] {
			return false
		}
	}
	return true
}

func Age(c *Cache) time.Duration {
	if c == nil || c.ScannedAt.IsZero() {
		return 0
	}
	return time.Since(c.ScannedAt)
}
