package shell

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

//go:embed devhub.zsh
var files embed.FS

const zshrcLine = `[[ -f ~/.devhub/shell/devhub.zsh ]] && source ~/.devhub/shell/devhub.zsh`

func StarshipSnippet() string {
	return `[env_var.DEVHUB_PROJECT]
format = '[\[$env_value\]](bold bg:#89B4FA fg:black) '
disabled = false
`
}

func DetectStarship() bool {
	if os.Getenv("STARSHIP_SHELL") != "" {
		return true
	}
	_, err := exec.LookPath("starship")
	return err == nil
}

func Install(devhubDir, home string) (zshrcPath string, wroteZshrc bool, starship bool, err error) {
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return "", false, false, err
		}
	}
	if devhubDir == "" {
		devhubDir = filepath.Join(home, ".devhub")
	}
	shellDir := filepath.Join(devhubDir, "shell")
	if err := os.MkdirAll(shellDir, 0o755); err != nil {
		return "", false, false, err
	}
	body, err := files.ReadFile("devhub.zsh")
	if err != nil {
		return "", false, false, err
	}
	if err := os.WriteFile(filepath.Join(shellDir, "devhub.zsh"), body, 0o644); err != nil {
		return "", false, false, err
	}

	starship = DetectStarship()
	zshrcPath = filepath.Join(home, ".zshrc")
	existing, err := os.ReadFile(zshrcPath)
	if err != nil && !os.IsNotExist(err) {
		return zshrcPath, false, starship, err
	}
	if strings.Contains(string(existing), zshrcLine) {
		return zshrcPath, false, starship, nil
	}
	if len(existing) > 0 {
		bak := zshrcPath + ".devhub.bak." + time.Now().Format("20060102150405")
		if err := os.WriteFile(bak, existing, 0o644); err != nil {
			return zshrcPath, false, starship, fmt.Errorf("backup .zshrc: %w", err)
		}
	}
	var b strings.Builder
	if len(existing) > 0 {
		b.Write(existing)
		if !strings.HasSuffix(string(existing), "\n") {
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	b.WriteString("# devhub prompt badge\n")
	b.WriteString(zshrcLine)
	b.WriteByte('\n')
	if err := os.WriteFile(zshrcPath, []byte(b.String()), 0o644); err != nil {
		return zshrcPath, false, starship, err
	}
	return zshrcPath, true, starship, nil
}
