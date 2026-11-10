package mcp

import (
	"regexp"
	"strings"
)

var secretRes = []*regexp.Regexp{
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`(?i)-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]+?-----END [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9\-\._~\+\/]+=*`),
	regexp.MustCompile(`(?i)(AIza[0-9A-Za-z\-_]{35})`),
	regexp.MustCompile(`(?i)("[A-Za-z0-9_]*SECRET[A-Za-z0-9_]*"\s*:\s*")[^"]+(")`),
	regexp.MustCompile(`(?i)\b([A-Z0-9_]*SECRET[A-Z0-9_]*)=([^\s]+)`),
	regexp.MustCompile(`(?i)\b([A-Z0-9_]*TOKEN[A-Z0-9_]*)=([^\s]+)`),
	regexp.MustCompile(`(?s)"type"\s*:\s*"service_account"`),
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
	return out
}
