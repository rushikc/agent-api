package scan

import "time"

type Options struct {
	Roots          []string
	Depth          int
	Nested         bool
	IncludeVolumes bool
	Exclude        []string
	Workers        int
	Projects       map[string]string
	Progress       func(Progress)
}

type Progress struct {
	Dirs    int           `json:"dirs"`
	Repos   int           `json:"repos"`
	Denied  int           `json:"denied"`
	Current string        `json:"current,omitempty"`
	Elapsed time.Duration `json:"elapsed_ms"`
	Privacy bool          `json:"privacy_denied,omitempty"`
}

type Result struct {
	Repos          []Repo        `json:"repos"`
	Dirs           int           `json:"dirs"`
	Denied         int           `json:"denied"`
	PrivacyDenied  bool          `json:"privacy_denied"`
	Roots          []string      `json:"roots"`
	Depth          int           `json:"depth"`
	Nested         bool          `json:"nested"`
	IncludeVolumes bool          `json:"include_volumes"`
	Duration       time.Duration `json:"duration_ms"`
	Partial        bool          `json:"partial"`
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
