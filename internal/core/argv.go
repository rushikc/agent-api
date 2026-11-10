package core

import (
	"encoding/json"
	"strings"

	"github.com/rushikesh/devhub/internal/registry"
)

func EncodeArgv(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	b, err := json.Marshal(argv)
	if err != nil {
		return ""
	}
	return string(b)
}

func ParseArgv(s registry.Session) []string {
	if s.ArgvJSON != "" {
		var argv []string
		if err := json.Unmarshal([]byte(s.ArgvJSON), &argv); err == nil && len(argv) > 0 {
			return argv
		}
	}
	if strings.TrimSpace(s.Command) == "" {
		return nil
	}
	if strings.HasPrefix(strings.TrimSpace(s.Command), "[") {
		var argv []string
		if err := json.Unmarshal([]byte(s.Command), &argv); err == nil && len(argv) > 0 {
			return argv
		}
	}
	return strings.Fields(s.Command)
}

func DisplayCommand(argv []string) string {
	return strings.Join(argv, " ")
}
