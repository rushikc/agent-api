package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/rushikesh/devhub/internal/client"
	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/daemon"
	"github.com/rushikesh/devhub/internal/janitor"
	"github.com/rushikesh/devhub/internal/mcp"
	"github.com/rushikesh/devhub/internal/profile"
	"github.com/rushikesh/devhub/internal/skills"
	"github.com/rushikesh/devhub/internal/ui"
	"github.com/spf13/cobra"
)

func withCaller(kind string) {
	_ = os.Setenv("DEVHUB_CALLER", kind)
}

func tryDaemon(cfg *config.Config) *client.Client {
	_ = daemon.Ensure(cfg)
	c := client.Unix(cfg, os.Getenv("DEVHUB_CALLER"))
	if c.Healthy() {
		return c
	}
	return nil
}

func newDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the local API daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := core.Open()
			if err != nil {
				return err
			}
			defer svc.Close()
			return daemon.Run(svc.Cfg, svc)
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			fmt.Println(daemon.Status(cfg))
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop the daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return daemon.Stop(cfg)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "restart",
		Short: "Restart the daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			_ = daemon.Stop(cfg)
			return daemon.Ensure(cfg)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "logs",
		Short: "Print daemon logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			out, err := daemon.Logs(cfg, 80)
			if err != nil {
				return err
			}
			fmt.Print(out)
			if !strings.HasSuffix(out, "\n") {
				fmt.Println()
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Install a launchd agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			p, err := daemon.InstallLaunchd(cfg)
			if err != nil {
				return err
			}
			fmt.Println("wrote", p)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "uninstall",
		Short: "Remove the launchd agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.UninstallLaunchd()
		},
	})
	return cmd
}

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP stdio server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			withCaller("mcp")
			return mcp.RunStdio(cfg)
		},
	}
	var cursor bool
	inst := &cobra.Command{
		Use:   "install",
		Short: "Merge the server into an MCP client config",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cursor {
				return fmt.Errorf("pass --cursor")
			}
			path, err := mcp.InstallCursor(mcp.MustExe())
			if err != nil {
				return err
			}
			fmt.Println("updated", path)
			return nil
		},
	}
	inst.Flags().BoolVar(&cursor, "cursor", false, "merge ~/.cursor/mcp.json")
	cmd.AddCommand(inst)
	return cmd
}

func newSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Skills hub",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: "List skills and rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if c := tryDaemon(cfg); c != nil {
				b, err := c.Skills()
				if err != nil {
					return err
				}
				fmt.Println(string(b))
				return nil
			}
			hub, err := skills.Open(cfg)
			if err != nil {
				return err
			}
			items, err := hub.List()
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "KIND\tNAME\tSCOPE\tENABLED")
			for _, it := range items {
				fmt.Fprintf(w, "%s\t%s\t%s\t%v\n", it.Kind, it.Name, it.Scope, it.Enabled)
			}
			return w.Flush()
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "new <name>",
		Args:  cobra.ExactArgs(1),
		Short: "Create a skill",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			hub, err := skills.Open(cfg)
			if err != nil {
				return err
			}
			p, err := hub.NewSkill(args[0], "global")
			if err != nil {
				return err
			}
			fmt.Println(p)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "get <name>",
		Args:  cobra.ExactArgs(1),
		Short: "Show a skill or rule",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if c := tryDaemon(cfg); c != nil {
				b, err := c.Skill(args[0])
				if err != nil {
					return err
				}
				fmt.Println(string(b))
				return nil
			}
			hub, err := skills.Open(cfg)
			if err != nil {
				return err
			}
			it, err := hub.Get(args[0])
			if err != nil {
				return err
			}
			fmt.Print(it.Body)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "sync",
		Short: "Sync adapters",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if c := tryDaemon(cfg); c != nil {
				return c.SyncSkills()
			}
			hub, err := skills.Open(cfg)
			if err != nil {
				return err
			}
			m, err := hub.Sync(false)
			if err != nil {
				return err
			}
			fmt.Printf("synced %d files\n", len(m.Files))
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "unsync",
		Short: "Remove managed adapter files",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			hub, err := skills.Open(cfg)
			if err != nil {
				return err
			}
			return hub.Unsync()
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "doctor",
		Short: "Check adapter health",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			hub, err := skills.Open(cfg)
			if err != nil {
				return err
			}
			for _, iss := range hub.Doctor() {
				fmt.Printf("%s\t%s\t%s\n", iss.Kind, iss.Path, iss.Message)
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "import <path>",
		Args:  cobra.ExactArgs(1),
		Short: "Import a SKILL.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			hub, err := skills.Open(cfg)
			if err != nil {
				return err
			}
			return hub.ImportFile(args[0], "")
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "cp <name>",
		Args:  cobra.ExactArgs(1),
		Short: "Copy skill body to clipboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			hub, err := skills.Open(cfg)
			if err != nil {
				return err
			}
			it, err := hub.Get(args[0])
			if err != nil {
				return err
			}
			c := exec.Command("pbcopy")
			c.Stdin = strings.NewReader(it.Body)
			return c.Run()
		},
	})
	return cmd
}

func newRulesCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "rules", Short: "Rules hub"}
	cmd.AddCommand(&cobra.Command{
		Use:   "new <name>",
		Args:  cobra.ExactArgs(1),
		Short: "Create a rule",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			hub, err := skills.Open(cfg)
			if err != nil {
				return err
			}
			p, err := hub.NewRule(args[0], "global")
			if err != nil {
				return err
			}
			fmt.Println(p)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: "List rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			hub, err := skills.Open(cfg)
			if err != nil {
				return err
			}
			items, err := hub.List()
			if err != nil {
				return err
			}
			for _, it := range items {
				if it.Kind == "rule" {
					fmt.Println(it.Name, it.Scope)
				}
			}
			return nil
		},
	})
	return cmd
}

func newUpCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "up [project]",
		Short: "Start a project profile",
		RunE: withService(func(svc *core.Service, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			if c := tryDaemon(svc.Cfg); c != nil && name != "" && yes {
				b, err := c.ProfileUp(name)
				if err != nil {
					return err
				}
				fmt.Println(string(b))
				return nil
			}
			pr, err := profile.Find(svc.Cfg, name)
			if err != nil {
				return err
			}
			if !yes {
				rows, _ := svc.DB.List(false)
				for _, sh := range pr.Shadows {
					if sh.Port == 0 {
						continue
					}
					if p, taken := janitor.PortTaken(sh.Port, rows, nil); taken {
						fmt.Printf("port %d in use by %s (pid %d). [f]ree / [s]kip / [a]bort? ", p.Port, p.Owner, p.PID)
						line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
						switch strings.ToLower(strings.TrimSpace(line)) {
						case "f", "y", "yes", "free":
							if err := janitor.KillTree(p.PID); err != nil {
								return err
							}
						case "s", "skip":
							continue
						default:
							return fmt.Errorf("aborted")
						}
					}
				}
			}
			res, err := profile.Up(svc, pr, yes)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tKIND\tPORT\tURL\tSTATUS")
			for _, r := range res {
				port := ""
				if r.Port > 0 {
					port = strconv.Itoa(r.Port)
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.Name, r.Kind, port, r.URL, r.Status)
			}
			return w.Flush()
		}),
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "free conflicting ports without asking")
	return cmd
}

func newDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down [project]",
		Short: "Stop a project profile",
		RunE: withService(func(svc *core.Service, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			if name == "" {
				pr, err := profile.Find(svc.Cfg, "")
				if err != nil {
					return err
				}
				name = pr.Name
			}
			if c := tryDaemon(svc.Cfg); c != nil {
				if err := c.ProfileDown(name); err == nil {
					fmt.Println("down", name)
					return nil
				}
			}
			if err := profile.Down(svc, name); err != nil {
				return err
			}
			fmt.Println("down", name)
			return nil
		}),
	}
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Write a starter devhub.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			pr, err := profile.Init(wd)
			if err != nil {
				return err
			}
			fmt.Println("wrote", wd+"/devhub.yaml for", pr.Name)
			return nil
		},
	}
}

func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "profile", Short: "Project profiles"}
	cmd.AddCommand(&cobra.Command{
		Use:   "validate [project]",
		Short: "Validate devhub.yaml",
		RunE: withServiceOpt(false, func(svc *core.Service, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			pr, err := profile.Find(svc.Cfg, name)
			if err != nil {
				return err
			}
			if err := profile.Validate(pr); err != nil {
				return err
			}
			fmt.Println("ok", pr.Name)
			return nil
		}),
	})
	return cmd
}

func newPortsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ports",
		Short: "List listening TCP ports",
		RunE: withServiceOpt(false, func(svc *core.Service, args []string) error {
			if c := tryDaemon(svc.Cfg); c != nil {
				b, err := c.Ports()
				if err == nil {
					fmt.Println(string(b))
					return nil
				}
			}
			rows, _ := svc.DB.List(false)
			ports, err := janitor.ListPorts(rows, nil)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "PORT\tPID\tOWNER\tCMD")
			for _, p := range ports {
				fmt.Fprintf(w, "%d\t%d\t%s\t%s\n", p.Port, p.PID, p.Owner, p.Cmd)
			}
			return w.Flush()
		}),
	}
}

func newFreeCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "free <port>",
		Args:  cobra.ExactArgs(1),
		Short: "Free a TCP port",
		RunE: withServiceOpt(false, func(svc *core.Service, args []string) error {
			port, err := strconv.Atoi(args[0])
			if err != nil {
				return err
			}
			rows, _ := svc.DB.List(false)
			p, taken := janitor.PortTaken(port, rows, nil)
			if !taken {
				return fmt.Errorf("nothing listening on :%d", port)
			}
			fmt.Printf(":%d pid=%d owner=%s cmd=%s\n", p.Port, p.PID, p.Owner, p.Cmd)
			if !yes {
				fmt.Print("kill? [y/N] ")
				line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
				if strings.ToLower(strings.TrimSpace(line)) != "y" {
					fmt.Println("skipped")
					return nil
				}
			}
			if c := tryDaemon(svc.Cfg); c != nil {
				if err := c.FreePort(port); err == nil {
					return nil
				}
			}
			return janitor.KillTree(p.PID)
		}),
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "do not confirm")
	return cmd
}

func newOrphansCmd() *cobra.Command {
	var kill bool
	cmd := &cobra.Command{
		Use:   "orphans",
		Short: "List orphan processes",
		RunE: withServiceOpt(false, func(svc *core.Service, args []string) error {
			rows, _ := svc.DB.List(false)
			orph, err := janitor.ListOrphans(svc.Cfg, rows, nil)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "PID\tNAME\tPROJECT\tCWD")
			for _, o := range orph {
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", o.PID, o.Name, o.Project, o.Cwd)
				if kill {
					_ = janitor.KillTree(o.PID)
				}
			}
			return w.Flush()
		}),
	}
	cmd.Flags().BoolVar(&kill, "kill", false, "SIGTERM matching orphans")
	return cmd
}

func newJanitorUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "janitor",
		Short: "Ports and orphans UI",
		RunE: withService(func(svc *core.Service, args []string) error {
			withCaller("tui")
			return ui.RunJanitor(svc)
		}),
	}
}

func newSkillsUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "skills-ui",
		Short: "Skills TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			withCaller("tui")
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			_ = daemon.Ensure(cfg)
			return ui.RunSkills()
		},
	}
}

func newUpdownUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "updown",
		Short: "Profile up/down UI",
		RunE: withService(func(svc *core.Service, args []string) error {
			withCaller("tui")
			return ui.RunUpDown(svc)
		}),
	}
}

