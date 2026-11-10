package scan

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/rushikesh/devhub/internal/jsonx"
)

func TestDurationJSONMilliseconds(t *testing.T) {
	var r Result
	r.DurationMS, r.Duration = setElapsed(2300 * time.Microsecond)
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	ms, ok := got["duration_ms"].(float64)
	if !ok || ms < 2.2 || ms > 2.4 {
		t.Fatalf("duration_ms %v in %s", got["duration_ms"], b)
	}
	if got["duration"] != "2.3ms" {
		t.Fatalf("duration %v", got["duration"])
	}
	if ms > 100 {
		t.Fatalf("looks like nanoseconds: %s", b)
	}
}

func TestFormatDuration(t *testing.T) {
	if jsonx.FormatDuration(2300*time.Microsecond) != "2.3ms" {
		t.Fatal(jsonx.FormatDuration(2300 * time.Microsecond))
	}
	if jsonx.FormatDuration(456*time.Millisecond) != "456ms" {
		t.Fatal(jsonx.FormatDuration(456 * time.Millisecond))
	}
	if jsonx.FormatDuration(1200*time.Millisecond) != "1.2s" {
		t.Fatal(jsonx.FormatDuration(1200 * time.Millisecond))
	}
}
