package scan

import (
	"time"

	"github.com/rushikesh/devhub/internal/jsonx"
)

type Options struct {
	Roots          []string
	Depth          int
	Nested         bool
	IncludeVolumes bool
	IncludeHidden  bool
	Exclude        []string
	Workers        int
	Projects       map[string]string
	Home           string
	AllowRoots     []string
	Progress       func(Progress)
}

type Progress struct {
	Dirs      int     `json:"dirs"`
	Repos     int     `json:"repos"`
	Denied    int     `json:"denied"`
	Current   string  `json:"current,omitempty"`
	ElapsedMS float64 `json:"elapsed_ms"`
	Elapsed   string  `json:"elapsed,omitempty"`
	Privacy   bool    `json:"privacy_denied,omitempty"`
}

type Result struct {
	Repos          []Repo   `json:"repos"`
	Dirs           int      `json:"dirs"`
	Denied         int      `json:"denied"`
	PrivacyDenied  bool     `json:"privacy_denied"`
	Roots          []string `json:"roots"`
	Depth          int      `json:"depth"`
	Nested         bool     `json:"nested"`
	IncludeVolumes bool     `json:"include_volumes"`
	DurationMS     float64  `json:"duration_ms"`
	Duration       string   `json:"duration"`
	Partial        bool     `json:"partial"`
}

func setElapsed(d time.Duration) (float64, string) {
	return jsonx.DurationMS(d), jsonx.FormatDuration(d)
}

type Repo struct {
	Name           string    `json:"name"`
	Path           string    `json:"path"`
	Branch         string    `json:"branch,omitempty"`
	Remote         string    `json:"remote,omitempty"`
	LastActivity   time.Time `json:"last_activity,omitempty"`
	Stack          []string  `json:"stack,omitempty"`
	HasDevhubYAML  bool      `json:"has_devhub_yaml"`
	Registered     bool      `json:"registered"`
	RegisteredName string    `json:"registered_name,omitempty"`
}

type Cache struct {
	ScannedAt      time.Time `json:"scanned_at"`
	Roots          []string  `json:"roots"`
	Depth          int       `json:"depth"`
	Nested         bool      `json:"nested"`
	IncludeVolumes bool      `json:"include_volumes"`
	Dirs           int       `json:"dirs"`
	Denied         int       `json:"denied"`
	PrivacyDenied  bool      `json:"privacy_denied"`
	Repos          []Repo    `json:"repos"`
}
