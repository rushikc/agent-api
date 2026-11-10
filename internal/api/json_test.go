package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestWriteJSONNilSliceIsArray(t *testing.T) {
	var orphans []string
	rec := httptest.NewRecorder()
	writeJSON(rec, orphans)
	var got any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err, rec.Body.String())
	}
	if _, ok := got.([]any); !ok {
		t.Fatalf("want array got %T %s", got, rec.Body.String())
	}
}
