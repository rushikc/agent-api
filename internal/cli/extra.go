package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/daemon"
	"github.com/rushikesh/devhub/internal/shell"
	"github.com/spf13/cobra"
)

func binName() string {
	return filepath.Base(os.Args[0])
}

func newShellCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Install the project prompt badge",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Install ~/.devhub/shell/devhub.zsh and source it from ~/.zshrc",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := config.Dir()
			if err != nil {
				return err
			}
			return runShellInstall(dir)
		},
	})
	return cmd
}

func runShellInstall(devhubDir string) error {
	zshrc, wrote, starship, err := shell.Install(devhubDir, "")
	if err != nil {
		return err
	}
	fmt.Println("wrote", filepath.Join(devhubDir, "shell", "devhub.zsh"))
	if wrote {
		fmt.Println("updated", zshrc, "(backup created)")
	} else {
		fmt.Println(zshrc, "already sources the hook")
	}
	if starship {
		fmt.Println()
		fmt.Println("Starship detected. Do not edit PROMPT; add this to ~/.config/starship.toml:")
		fmt.Println()
		fmt.Print(shell.StarshipSnippet())
	}
	return nil
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check tmux, Option-as-Meta, and PATH",
		RunE: func(cmd *cobra.Command, args []string) error {
			term := os.Getenv("TERM_PROGRAM")
			fmt.Printf("TERM_PROGRAM=%s\n", term)
			if cfg, err := config.Load(); err == nil {
				fmt.Println("daemon:", daemon.Status(cfg))
			}
			fmt.Println()
			fmt.Println("Option (Alt) must send Meta/Esc+ so M-s, M-h, M-p work.")
			fmt.Println("This disables typing special characters such as ©/π with Option in that profile.")
			fmt.Println()
			switch term {
			case "iTerm.app":
				printITermHelp()
				fmt.Println("Or run:", binName(), "setup iterm")
			case "Apple_Terminal":
				printTerminalHelp()
				fmt.Println("Or run:", binName(), "setup terminal")
			default:
				printITermHelp()
				fmt.Println()
				printTerminalHelp()
			}
			return nil
		},
	}
}

func printITermHelp() {
	fmt.Println("iTerm2:")
	fmt.Println("  Profiles > Keys > Left Option key = Esc+")
}

func printTerminalHelp() {
	fmt.Println("Terminal.app:")
	fmt.Println("  Settings > Profiles > Keyboard > Use Option as Meta key")
}

func newSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure the terminal so Option sends Meta",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "iterm",
		Short: "Write an iTerm2 Dynamic Profile that sets Left Option = Esc+",
		RunE: func(cmd *cobra.Command, args []string) error {
			return setupITerm()
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "terminal",
		Short: "Set Option as Meta on the Terminal.app default profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			return setupTerminal()
		},
	})
	return cmd
}

func setupITerm() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, "Library", "Application Support", "iTerm2", "DynamicProfiles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "devhub.json")
	payload := map[string]any{
		"Profiles": []map[string]any{
			{
				"Name":                   "devhub",
				"Guid":                   "8f3c0e1a-devh-ub00-opti-onkey0000001",
				"Left Option Key Sends":  2,
				"Right Option Key Sends": 2,
			},
		},
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	fmt.Println("wrote", path)
	fmt.Println("In iTerm2, select the \"devhub\" profile (or copy Left Option = Esc+ into your usual profile).")
	fmt.Println("Note: Option as Esc+ disables special characters such as ©/π in that profile.")
	return nil
}

func setupTerminal() error {
	fmt.Print("Set Option as Meta for Terminal.app default profile? [y/N] ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	if line != "y" && line != "yes" {
		fmt.Println("skipped")
		return nil
	}
	script := `tell application "Terminal"
  set use option as meta key of default settings to true
end tell`
	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("osascript: %w\n%s", err, out)
	}
	fmt.Println("Option as Meta enabled for Terminal.app default profile.")
	fmt.Println("Note: this disables special characters such as ©/π with Option in that profile.")
	return nil
}
