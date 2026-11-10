package gcp

import (
	"strconv"
	"strings"
)

func ParseBytes(s string) int64 {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 1 << 30
	}
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "GIB"):
		mult = 1 << 30
		s = strings.TrimSuffix(s, "GIB")
	case strings.HasSuffix(s, "GB"):
		mult = 1e9
		s = strings.TrimSuffix(s, "GB")
	case strings.HasSuffix(s, "MIB"):
		mult = 1 << 20
		s = strings.TrimSuffix(s, "MIB")
	case strings.HasSuffix(s, "MB"):
		mult = 1e6
		s = strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "KIB"):
		mult = 1 << 10
		s = strings.TrimSuffix(s, "KIB")
	case strings.HasSuffix(s, "KB"):
		mult = 1e3
		s = strings.TrimSuffix(s, "KB")
	}
	s = strings.TrimSpace(s)
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 1 << 30
	}
	return int64(n * float64(mult))
}
