package jsonx

import (
	"bytes"
	"encoding/json"
	"testing"
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
