package skills

import (
	"strings"
	"testing"
)

func TestToMDCGolden(t *testing.T) {
	got := ToMDC(RuleMeta{Description: "Go style", Globs: "**/*.go", AlwaysApply: true}, "Use gofmt.\n")
	for _, want := range []string{
		"---",
		"description: Go style",
		"globs: **/*.go",
		"alwaysApply: true",
		ManagedHeader,
		"Use gofmt.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestParseSkillRequiresName(t *testing.T) {
	_, _, err := ParseSkill("---\ndescription: x\n---\n")
	if err == nil {
		t.Fatal("expected error")
	}
	m, _, err := ParseSkill("---\nname: go-style\nscope: global\n---\nHi\n")
	if err != nil || m.Name != "go-style" {
		t.Fatal(err, m)
	}
}
