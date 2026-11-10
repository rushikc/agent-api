package jsonx

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestNormalizeNilSlice(t *testing.T) {
	var a []string
	b, err := Compact(a)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "[]" {
		t.Fatalf("%s", b)
	}
	var buf bytes.Buffer
	if err := Encode(&buf, []int(nil)); err != nil {
		t.Fatal(err)
	}
	var got any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got.([]any); !ok {
		t.Fatalf("%T", got)
	}
}

func ptrInt(n int) *int          { return &n }
func ptrString(s string) *string { return &s }

type inner struct {
	N *int
}

type row struct {
	ExitCode *int
	Name     *string
	Kids     []inner
	Nested   inner
	When     time.Time
	Tags     map[string]int
	Empty    []string
}

func TestNormalizeTable(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		in    any
		check func(t *testing.T, b []byte)
	}{
		{
			name: "nil *int",
			in:   row{ExitCode: nil},
			check: func(t *testing.T, b []byte) {
				if !bytes.Contains(b, []byte(`"ExitCode":null`)) && !bytes.Contains(b, []byte(`"ExitCode": null`)) {
					t.Fatalf("%s", b)
				}
			},
		},
		{
			name: "set *int",
			in:   row{ExitCode: ptrInt(7)},
			check: func(t *testing.T, b []byte) {
				if !bytes.Contains(b, []byte(`"ExitCode":7`)) && !bytes.Contains(b, []byte(`"ExitCode": 7`)) {
					t.Fatalf("%s", b)
				}
			},
		},
		{
			name: "nil *string",
			in:   row{Name: nil},
			check: func(t *testing.T, b []byte) {
				mustJSON(t, b)
			},
		},
		{
			name: "set *string",
			in:   row{Name: ptrString("ok")},
			check: func(t *testing.T, b []byte) {
				if !bytes.Contains(b, []byte(`"ok"`)) {
					t.Fatalf("%s", b)
				}
			},
		},
		{
			name: "nested struct pointer",
			in:   row{Nested: inner{N: ptrInt(3)}},
			check: func(t *testing.T, b []byte) {
				mustJSON(t, b)
			},
		},
		{
			name: "slice of structs with pointers",
			in:   []row{{ExitCode: ptrInt(1)}, {ExitCode: nil}},
			check: func(t *testing.T, b []byte) {
				mustJSON(t, b)
			},
		},
		{
			name: "map left intact",
			in:   row{Tags: map[string]int{"a": 1}},
			check: func(t *testing.T, b []byte) {
				if !bytes.Contains(b, []byte(`"a":1`)) && !bytes.Contains(b, []byte(`"a": 1`)) {
					t.Fatalf("%s", b)
				}
			},
		},
		{
			name: "time.Time",
			in:   row{When: now},
			check: func(t *testing.T, b []byte) {
				if !bytes.Contains(b, []byte("2026-11-10")) {
					t.Fatalf("%s", b)
				}
			},
		},
		{
			name: "struct time.Duration as milliseconds not nanoseconds",
			in:   struct{ Wait time.Duration }{Wait: 2300 * time.Microsecond},
			check: func(t *testing.T, b []byte) {
				if bytes.Contains(b, []byte("2300000")) {
					t.Fatalf("nanoseconds: %s", b)
				}
				var got struct{ Wait float64 }
				if err := json.Unmarshal(b, &got); err != nil {
					t.Fatal(err, string(b))
				}
				if got.Wait < 2.2 || got.Wait > 2.4 {
					t.Fatalf("%s", b)
				}
			},
		},
		{
			name: "time.Duration as milliseconds not nanoseconds",
			in:   2300 * time.Microsecond,
			check: func(t *testing.T, b []byte) {
				var ms float64
				if err := json.Unmarshal(b, &ms); err != nil {
					t.Fatal(err, string(b))
				}
				if ms < 2.2 || ms > 2.4 {
					t.Fatalf("%s", b)
				}
			},
		},
		{
			name: "nil nested slice becomes []",
			in:   row{Empty: nil},
			check: func(t *testing.T, b []byte) {
				if !bytes.Contains(b, []byte(`"Empty":[]`)) && !bytes.Contains(b, []byte(`"Empty": []`)) {
					t.Fatalf("%s", b)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := Compact(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			mustJSON(t, b)
			tc.check(t, b)
		})
	}
}

func mustJSON(t *testing.T, b []byte) {
	t.Helper()
	var got any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err, string(b))
	}
}
