package mcp

import "testing"

func TestScrub(t *testing.T) {
	in := "AKIAIOSFODNN7EXAMPLE Bearer abcdef.gh TOKEN_SECRET=wow AWS_TOKEN=xyz\n-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n"
	out := Scrub(in)
	if contains(out, "IOSFODNN7") || contains(out, "abcdef") || contains(out, "wow") {
		t.Fatal(out)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
