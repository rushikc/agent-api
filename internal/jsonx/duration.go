package jsonx

import (
	"fmt"
	"math"
	"time"
)

func DurationMS(d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	ms := float64(d) / float64(time.Millisecond)
	return math.Round(ms*10) / 10
}

func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	ms := float64(d) / float64(time.Millisecond)
	switch {
	case ms < 1000:
		if ms == math.Trunc(ms) && ms >= 10 {
			return fmt.Sprintf("%.0fms", ms)
		}
		return fmt.Sprintf("%.1fms", ms)
	default:
		s := ms / 1000
		if s == math.Trunc(s) {
			return fmt.Sprintf("%.0fs", s)
		}
		return fmt.Sprintf("%.1fs", s)
	}
}
