package mcp

import "github.com/rushikesh/devhub/internal/scrub"

func SetExactValues(vals []string) { scrub.SetExactValues(vals) }

func ExactValues() []string { return scrub.ExactValues() }

func Scrub(s string) string { return scrub.Scrub(s) }
