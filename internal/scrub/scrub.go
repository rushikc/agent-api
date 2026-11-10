package scrub

import (
	"regexp"
	"strings"
	"sync"
)

var secretRes = []*regexp.Regexp{
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`(?i)-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]+?-----END [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9\-\._~\+\/]+=*`),
	regexp.MustCompile(`(?i)(AIza[0-9A-Za-z\-_]{35})`),
	regexp.MustCompile(`(?i)("[A-Za-z0-9_]*SECRET[A-Za-z0-9_]*"\s*:\s*")[^"]+(")`),
	regexp.MustCompile(`(?i)\b([A-Z0-9_]*SECRET[A-Z0-9_]*)=([^\s"]+)`),
	regexp.MustCompile(`(?i)\b([A-Z0-9_]*TOKEN[A-Z0-9_]*)=([^\s"]+)`),
	regexp.MustCompile(`(?s)"type"\s*:\s*"service_account"`),
}

var (
	exactMu     sync.RWMutex
	exactValues []string
)

func SetExactValues(vals []string) {
	exactMu.Lock()
	defer exactMu.Unlock()
	exactValues = uniqueNonEmpty(vals)
}

func ExactValues() []string {
	exactMu.RLock()
	defer exactMu.RUnlock()
	return append([]string(nil), exactValues...)
}

func Scrub(s string) string {
	out := s
	out = secretRes[0].ReplaceAllString(out, "AKIA****************")
	out = secretRes[1].ReplaceAllString(out, "[REDACTED PRIVATE KEY]")
	out = secretRes[2].ReplaceAllString(out, "Bearer [REDACTED]")
	out = secretRes[3].ReplaceAllString(out, "[REDACTED GCP KEY]")
	out = secretRes[4].ReplaceAllString(out, `${1}[REDACTED]$2`)
	out = secretRes[5].ReplaceAllString(out, `${1}=[REDACTED]`)
	out = secretRes[6].ReplaceAllString(out, `${1}=[REDACTED]`)
	if strings.Contains(out, `"service_account"`) {
		out = secretRes[7].ReplaceAllString(out, `"type": "service_account" [REDACTED]`)
	}
	exactMu.RLock()
	vals := append([]string(nil), exactValues...)
	exactMu.RUnlock()
	for _, v := range vals {
		if v != "" && strings.Contains(out, v) {
			out = strings.ReplaceAll(out, v, "[REDACTED]")
		}
	}
	return out
}

func uniqueNonEmpty(vals []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, v := range vals {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
