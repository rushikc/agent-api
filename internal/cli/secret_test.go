package cli

import (
	"strings"
	"testing"
)

func TestReadSecretValueNonTTY(t *testing.T) {
	b, err := readSecretValue(strings.NewReader("  supersecret\n"), false)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "supersecret" {
		t.Fatalf("%q", b)
	}
}
