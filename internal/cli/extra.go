package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"syscall"

	"github.com/rushikesh/devhub/internal/client"
	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/daemon"
	"github.com/rushikesh/devhub/internal/e2e"
	"github.com/rushikesh/devhub/internal/monitor"
	"github.com/rushikesh/devhub/internal/shell"
	"github.com/rushikesh/devhub/internal/tmux"
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
	var fix bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check tmux, Option-as-Meta, and PATH",
		RunE: func(cmd *cobra.Command, args []string) error {
			term := os.Getenv("TERM_PROGRAM")
			fmt.Printf("TERM_PROGRAM=%s\n", term)
			if cfg, err := config.Load(); err == nil {
				fmt.Println("daemon:", daemon.Status(cfg))
				printDoctorChecks(cfg)
				if fix {
					if err := doctorFixProjects(cfg); err != nil {
						return err
					}
				}
			}
			printV3Doctor()
			fmt.Println()
			fmt.Println("Option (Alt) must send Meta/Esc+ so M-s, M-h, M-p, M-P work.")
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
	cmd.Flags().BoolVar(&fix, "fix", false, "offer to fix missing project paths")
	return cmd
}

func doctorFixProjects(cfg *config.Config) error {
	changed := false
	for name, p := range cfg.Projects {
		if _, err := os.Stat(p); err == nil {
			continue
		}
		hints := config.SuggestNearby(p)
		fmt.Printf("project %s: MISSING %s\n", name, p)
		if len(hints) > 0 {
			fmt.Printf("  suggestions: %s\n", strings.Join(hints, ", "))
			if confirmYes(fmt.Sprintf("  use %s?", hints[0]), false) {
				cfg.Projects[name] = hints[0]
				changed = true
				fmt.Println("  updated", name, "->", hints[0])
				continue
			}
		}
		if confirmYes("  remove this project from config?", false) {
			delete(cfg.Projects, name)
			changed = true
			fmt.Println("  removed", name)
		}
	}
	if !changed {
		return nil
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	if c := tryDaemon(cfg); c != nil {
		_ = c.Reload()
	}
	return nil
}

func printDoctorChecks(cfg *config.Config) {
	cli := tmuxCLI()
	if v, err := cli.Version(); err != nil {
		fmt.Println("tmux:", err)
	} else {
		maj, min, perr := tmuxparse(v)
		ok := perr == nil && tmux.VersionOK(maj, min)
		fmt.Printf("tmux: %s (need >= 3.3) ok=%v\n", v, ok)
	}
	path := os.Getenv("PATH")
	home, _ := os.UserHomeDir()
	inPath := strings.Contains(path, filepath.Join(home, "bin")) || strings.Contains(path, filepath.Join(home, ".local", "bin"))
	fmt.Printf("PATH has install dir (~/bin or ~/.local/bin): %v\n", inPath)
	fmt.Println("CGO_ENABLED:", os.Getenv("CGO_ENABLED"))
	for name, p := range cfg.Projects {
		if _, err := os.Stat(p); err != nil {
			fmt.Printf("project %s: MISSING %s\n", name, p)
		} else {
			fmt.Printf("project %s: %s\n", name, config.CanonPath(p))
		}
	}
	out, _ := exec.Command("ps", "-ax", "-o", "pid=,command=").Output()
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "devhub monitor") && !strings.Contains(line, "devhub daemon") {
			fmt.Println("stale process:", strings.TrimSpace(line), "(stop with: kill <pid>; daemon is the status.json writer)")
		}
	}
	procs, err := monitor.ReadPSErr()
	if err != nil {
		fmt.Println("process table:", err)
	} else {
		fmt.Printf("process table: %d rows\n", len(procs))
	}
	if stderr := monitor.LastPSStderr(); stderr != "" {
		fmt.Println("ps stderr:", stderr)
	}
}

func tmuxCLI() *tmux.CLI {
	return tmux.New()
}

func tmuxparse(v string) (int, int, error) {
	return tmux.ParseVersion(v)
}

func newExecEnvCmd() *cobra.Command {
	var id int64
	cmd := &cobra.Command{
		Use:    "exec-env",
		Short:  "Inject resolved secrets and exec a command",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if id == 0 {
				return fmt.Errorf("--id is required")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := daemon.Ensure(cfg); err != nil {
				return err
			}
			c := client.Unix(cfg, "exec-env")
			p, err := c.ExecPayload(id)
			if err != nil {
				return err
			}
			for k, v := range p.Env {
				_ = os.Setenv(k, v)
			}
			argv := p.Argv
			if len(args) > 0 {
				argv = args
			}
			if len(argv) == 0 {
				return fmt.Errorf("no command stored for session %d", id)
			}
			bin, err := exec.LookPath(argv[0])
			if err != nil {
				bin = argv[0]
			}
			return syscall.Exec(bin, argv, os.Environ())
		},
	}
	cmd.Flags().Int64Var(&id, "id", 0, "session id")
	return cmd
}

func newReloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reload",
		Short: "Reload tmux keybindings into -L devhub",
		RunE: withServiceOpt(false, func(svc *core.Service, args []string) error {
			err := svc.ApplyKeybindingsForce(true)
			if err == nil {
				n := core.CountBindKeys(core.RenderTmuxConf(svc.Cfg, svc.Executable()))
				fmt.Printf("reloaded %d bindings into running server\n", n)
				return nil
			}
			if errors.Is(err, tmux.ErrNoServer) {
				fmt.Println("bindings written; they'll load when you run dev")
				return nil
			}
			return err
		}),
	}
}

func printV3Doctor() {
	name := e2eRuntime()
	if err := e2e.Ping(); err != nil {
		fmt.Printf("docker: %s installed but not running (%s)\n", name, err)
	} else {
		fmt.Println("docker:", name)
	}
	if path, err := exec.LookPath("gcloud"); err == nil {
		fmt.Println("gcloud:", path)
	} else {
		fmt.Println("gcloud: not found")
	}
	cmd := exec.Command("gcloud", "auth", "application-default", "print-access-token")
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("ADC: missing (gcloud auth application-default login)")
	} else if len(strings.TrimSpace(string(out))) > 0 {
		fmt.Println("ADC: ok")
	}
	if _, err := exec.LookPath("terminal-notifier"); err == nil {
		fmt.Println("terminal-notifier: ok (click-to-focus enabled)")
	} else {
		fmt.Println("terminal-notifier: not found (osascript fallback, no click-through)")
	}
	if _, err := exec.LookPath("op"); err == nil {
		fmt.Println("1Password CLI: ok")
	} else {
		fmt.Println("1Password CLI: not installed")
	}
	if _, err := exec.LookPath("security"); err == nil {
		fmt.Println("Keychain (security CLI): ok, service=devhub")
	} else {
		fmt.Println("Keychain: security CLI not found")
	}
}

func e2eRuntime() string {
	return e2e.DetectRuntime()
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
