package scan

import (
	"os"
	"path/filepath"
)

func detectStack(dir string) (stack []string, hasDevhub bool) {
	exists := func(name string) bool {
		_, err := os.Lstat(filepath.Join(dir, name))
		return err == nil
	}
	if exists("go.mod") {
		stack = append(stack, "go")
	}
	if exists("package.json") {
		stack = append(stack, "node")
	}
	if exists("pyproject.toml") || exists("requirements.txt") {
		stack = append(stack, "python")
	}
	if exists("Cargo.toml") {
		stack = append(stack, "rust")
	}
	if exists("pom.xml") || exists("build.gradle") || exists("build.gradle.kts") {
		stack = append(stack, "java")
	}
	if exists("Gemfile") {
		stack = append(stack, "ruby")
	}
	hasDevhub = exists("devhub.yaml")
	if hasDevhub {
		stack = append(stack, "devhub")
	}
	return stack, hasDevhub
}
