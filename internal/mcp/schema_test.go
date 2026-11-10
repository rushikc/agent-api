package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStartShadowRequiredFields(t *testing.T) {
	rt := reflect.TypeOf(StartShadowArgs{})
	var required, optional []string
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag := f.Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if strings.Contains(tag, "omitempty") {
			optional = append(optional, name)
		} else {
			required = append(required, name)
		}
	}
	if strings.Join(required, ",") != "project,command" {
		t.Fatalf("required %v", required)
	}
	for _, n := range optional {
		if n == "project" || n == "command" {
			t.Fatal("project/command must not be omitempty")
		}
	}
}

func TestInstallCursorDryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	orig := []byte("{\n  \"mcpServers\": {}\n}\n")
	if err := os.WriteFile(path, orig, 0o644); err != nil {
		t.Fatal(err)
	}
	diff, err := InstallCursorAt(path, "/bin/devhub", true)
	if err != nil {
		t.Fatal(err)
	}
	if diff == "" || !strings.Contains(diff, "devhub") {
		t.Fatalf("diff %q", diff)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(orig) {
		t.Fatalf("file changed:\n%s", got)
	}
	_ = json.Valid(orig)
}
