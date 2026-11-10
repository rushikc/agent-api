package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func requiredJSON(t *testing.T, v any) []string {
	t.Helper()
	rt := reflect.TypeOf(v)
	var required []string
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag := f.Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		if !strings.Contains(tag, "omitempty") {
			required = append(required, name)
		}
	}
	return required
}

func TestV3ToolRequiredFields(t *testing.T) {
	cases := []struct {
		v    any
		want string
	}{
		{GCPLogsArgs{}, "alias"},
		{GCPBQArgs{}, "alias,sql"},
		{E2ERunArgs{}, "project"},
		{E2EIDArgs{}, "id"},
		{EmptyArgs{}, ""},
	}
	for _, tc := range cases {
		got := strings.Join(requiredJSON(t, tc.v), ",")
		if got != tc.want {
			t.Fatalf("%T required %q want %q", tc.v, got, tc.want)
		}
	}
}

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
	if strings.Join(required, ",") != "project" {
		t.Fatalf("required %v", required)
	}
	opt := strings.Join(optional, ",")
	if !strings.Contains(opt, "command") || !strings.Contains(opt, "argv") {
		t.Fatalf("optional %v", optional)
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
