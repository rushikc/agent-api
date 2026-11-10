package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/monitor"
	"github.com/rushikesh/devhub/internal/tmux"
	"github.com/rushikesh/devhub/internal/ui"
	"github.com/spf13/cobra"
)

func Execute() {
	if err := newRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, binName()+":", err)
		os.Exit(1)
	}
}

func newRoot() *cobra.Command {
	cmd := &cobra.Command{
		Use:           binName(),
		Short:         "Manage terminal sessions from one place",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := core.Open()
			if err != nil {
				return err
			}
			defer svc.Close()
			return svc.EnsureMain()
		},
	}
	cmd.AddCommand(
		newNewCmd(),
		newShadowCmd(),
		newLsCmd(),
		newPeekCmd(),
		newAttachCmd(),
		newKillCmd(),
		newRestartCmd(),
		newPickerCmd(),
		newShadowsCmd(),
		newTopCmd(),
		newProjectsUICmd(),
		newProjectCmd(),
		newStatusCmd(),
		newMonitorCmd(),
		newJumpCmd(),
		newShellCmd(),
		newDoctorCmd(),
		newSetupCmd(),
	)
	return cmd
}

func withService(fn func(*core.Service, []string) error) func(*cobra.Command, []string) error {
	return withServiceOpt(true, fn)
}

func withServiceOpt(needTmux bool, fn func(*core.Service, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		svc, err := core.Open()
		if err != nil {
			return err
		}
		defer svc.Close()
		if needTmux {
			if err := svc.Tmux.EnsureServer(); err != nil {
				return err
			}
		}
		return fn(svc, args)
	}
}

func newNewCmd() *cobra.Command {
	var project, name string
	cmd := &cobra.Command{
		Use:   "new",
		Short: "Create a new active terminal",
		RunE: withService(func(svc *core.Service, args []string) error {
			sess, err := svc.NewActive(project, name)
			if err != nil {
				return err
			}
			fmt.Printf("created %s/%s (%s)\n", sess.Project, sess.Name, sess.TmuxTarget)
			return nil
		}),
	}
	cmd.Flags().StringVar(&project, "project", "", "project name")
	cmd.Flags().StringVar(&name, "name", "", "window name (default next vN)")
	return cmd
}

func newShadowCmd() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "shadow <project> -- <command...>",
		Short: "Start a background (shadow) process",
		Args:  cobra.MinimumNArgs(1),
		RunE: withService(func(svc *core.Service, args []string) error {
			project := args[0]
			command := args[1:]
			sess, err := svc.NewShadow(project, name, command)
			if err != nil {
				return err
			}
			fmt.Printf("shadow %s/%s started (%s)\n", sess.Project, sess.Name, sess.Command)
			return nil
		}),
	}
	cmd.Flags().StringVar(&name, "name", "", "shadow name")
	return cmd
}

func newLsCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List sessions",
		RunE: withService(func(svc *core.Service, args []string) error {
			rows, err := svc.List(all)
			if err != nil {
				return err
			}
			// Second sample so CPU% is non-zero when processes are busy.
			time.Sleep(200 * time.Millisecond)
			rows, err = svc.List(all)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "PROJECT\tNAME\tKIND\tSTATUS\tCPU%\tRSS\tCOMMAND")
			for _, r := range rows {
				s := r.Session
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%.1f\t%s\t%s\n",
					s.Project, s.Name, s.Kind, s.Status, r.CPUPct, formatRSS(r.RSSBytes), s.Command)
			}
			return w.Flush()
		}),
	}
	cmd.Flags().BoolVar(&all, "all", false, "include exited sessions")
	return cmd
}

func formatRSS(n uint64) string {
	if n >= 1024*1024*1024 {
		return fmt.Sprintf("%.1fGB", float64(n)/(1024*1024*1024))
	}
	if n >= 1024*1024 {
		return fmt.Sprintf("%.0fMB", float64(n)/(1024*1024))
	}
	if n >= 1024 {
		return fmt.Sprintf("%.0fKB", float64(n)/1024)
	}
	return fmt.Sprintf("%dB", n)
}

func newPeekCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "peek <name>",
		Short: "Show the last 200 lines of a shadow pane",
		Args:  cobra.ExactArgs(1),
		RunE: withService(func(svc *core.Service, args []string) error {
			out, err := svc.Peek(args[0])
			if err != nil {
				return err
			}
			fmt.Print(out)
			if !strings.HasSuffix(out, "\n") {
				fmt.Println()
			}
			return nil
		}),
	}
}

func newAttachCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "attach <name>",
		Short: "Switch to a terminal window",
		Args:  cobra.ExactArgs(1),
		RunE: withService(func(svc *core.Service, args []string) error {
			return svc.Attach(args[0])
		}),
	}
}

func newKillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kill <name>",
		Short: "Kill a terminal",
		Args:  cobra.ExactArgs(1),
		RunE: withService(func(svc *core.Service, args []string) error {
			return svc.Kill(args[0])
		}),
	}
}

func newRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart <name>",
		Short: "Restart a shadow (or command) in the same cwd",
		Args:  cobra.ExactArgs(1),
		RunE: withService(func(svc *core.Service, args []string) error {
			return svc.Restart(args[0])
		}),
	}
}

func newPickerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "picker",
		Short: "Fuzzy picker for active terminals",
		RunE: withService(func(svc *core.Service, args []string) error {
			_ = svc.Reconcile()
			return ui.RunPicker(svc)
		}),
	}
}

func newShadowsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shadows",
		Short: "Shadow process UI",
		RunE: withService(func(svc *core.Service, args []string) error {
			_ = svc.Reconcile()
			return ui.RunShadows(svc)
		}),
	}
}

func newTopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "top",
		Short: "Live CPU and RAM table",
		RunE: withService(func(svc *core.Service, args []string) error {
			_ = svc.Reconcile()
			return ui.RunTop(svc)
		}),
	}
}

func newProjectsUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "projects",
		Short: "Project switcher",
		RunE: withService(func(svc *core.Service, args []string) error {
			return ui.RunProjects(svc)
		}),
	}
}

func newProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "add <name> <path>",
		Short: "Register a project directory",
		Args:  cobra.ExactArgs(2),
		RunE: withServiceOpt(false, func(svc *core.Service, args []string) error {
			if err := svc.Cfg.AddProject(args[0], args[1]); err != nil {
				return err
			}
			fmt.Printf("added project %s -> %s\n", args[0], svc.Cfg.Projects[args[0]])
			return nil
		}),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: "List projects",
		RunE: withServiceOpt(false, func(svc *core.Service, args []string) error {
			if len(svc.Cfg.Projects) == 0 {
				fmt.Println("no projects (devhub project add <name> <path>)")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tPATH")
			for n, p := range svc.Cfg.Projects {
				fmt.Fprintf(w, "%s\t%s\n", n, p)
			}
			return w.Flush()
		}),
	})
	return cmd
}

func newStatusCmd() *cobra.Command {
	var short bool
	cmd := &cobra.Command{
		Use:    "status",
		Short:  "Print cached resource snapshot",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := core.Open()
			if err != nil {
				if short {
					fmt.Print("0 active | 0 shadow | 0.0% | 0 B")
					return nil
				}
				return err
			}
			defer svc.Close()
			snap, err := monitor.ReadSnapshot(svc.Cfg.StatusPath())
			if err != nil {
				snap = monitor.Snapshot{}
			}
			if short {
				fmt.Print(monitor.FormatShort(snap))
				return nil
			}
			fmt.Printf("%s\n", monitor.FormatShort(snap))
			return nil
		},
	}
	cmd.Flags().BoolVar(&short, "short", false, "one-line status for tmux")
	return cmd
}

func newMonitorCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "monitor",
		Short:  "Background sampler (writes status.json)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := core.Open()
			if err != nil {
				return err
			}
			defer svc.Close()
			lock, err := monitor.TryLock(svc.Cfg.MonitorLockPath())
			if err != nil {
				return nil
			}
			defer lock.Close()
			client := svc.Tmux
			if _, ok := client.(*tmux.CLI); !ok {
				client = tmux.New()
			}
			return monitor.RunLoop(client, svc.DB, svc.Cfg.StatusPath(), 2*time.Second, nil)
		},
	}
}

func newJumpCmd() *cobra.Command {
	var session string
	cmd := &cobra.Command{
		Use:    "jump <n>",
		Short:  "Jump to vN of the current project",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: withService(func(svc *core.Service, args []string) error {
			n, err := core.ParseJumpN(args[0])
			if err != nil {
				return err
			}
			return svc.Jump(n, session)
		}),
	}
	cmd.Flags().StringVar(&session, "session", "", "tmux session name")
	return cmd
}
