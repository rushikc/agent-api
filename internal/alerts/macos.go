package alerts

import (
	"os/exec"
	"strconv"
)

type MacOS struct{}

func (MacOS) Name() string { return "macos" }

func (MacOS) Notify(n Notification) error {
	if _, err := exec.LookPath("terminal-notifier"); err == nil {
		args := []string{"-title", n.Title, "-message", n.Message}
		if n.Sound {
			args = append(args, "-sound", "default")
		}
		if n.Execute != "" {
			args = append(args, "-execute", n.Execute)
		}
		cmd := exec.Command("terminal-notifier", args...)
		return cmd.Run()
	}
	script := `display notification ` + strconv.Quote(n.Message) + ` with title ` + strconv.Quote(n.Title)
	if n.Sound {
		script += ` sound name "default"`
	}
	return exec.Command("osascript", "-e", script).Run()
}

func TerminalNotifierInstalled() bool {
	_, err := exec.LookPath("terminal-notifier")
	return err == nil
}
