package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rushikesh/devhub/internal/client"
	"github.com/rushikesh/devhub/internal/config"
	"github.com/rushikesh/devhub/internal/e2e"
	"github.com/rushikesh/devhub/internal/profile"
	"github.com/rushikesh/devhub/internal/ui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func mustDaemon() (*client.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	c := tryDaemon(cfg)
	if c == nil {
		return nil, fmt.Errorf("daemon is not healthy")
	}
	return c, nil
}

func newGCPCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "gcp", Short: "GCP gateway"}
	cmd.AddCommand(&cobra.Command{
		Use: "ls", Short: "List aliases",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			b, err := c.GCPProjects()
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		},
	})
	var filter, since, sev string
	var limit int
	var yes bool
	logs := &cobra.Command{
		Use: "logs <alias>", Short: "Cloud Logging",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			b, err := c.GCPCall(args[0], "logs.read", map[string]any{"filter": filter, "since": since, "severity": sev, "limit": limit}, yes, false)
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		},
	}
	logs.Flags().StringVar(&filter, "filter", "", "logging filter")
	logs.Flags().StringVar(&since, "since", "1h", "duration")
	logs.Flags().StringVar(&sev, "severity", "", "min severity")
	logs.Flags().IntVar(&limit, "limit", 100, "max entries")
	logs.Flags().BoolVarP(&yes, "yes", "y", false, "confirm prod")
	cmd.AddCommand(logs)

	var bqYes bool
	cmd.AddCommand(&cobra.Command{
		Use: "bq <alias> <sql>", Short: "SELECT-only BigQuery", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			b, err := c.GCPCall(args[0], "bq.query", map[string]any{"sql": args[1]}, bqYes, false)
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		},
	})
	cmd.Commands()[len(cmd.Commands())-1].Flags().BoolVarP(&bqYes, "yes", "y", false, "confirm prod")

	var runYes bool
	run := &cobra.Command{
		Use: "run <alias>", Short: "Cloud Run status", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			b, err := c.GCPCall(args[0], "run.status", map[string]any{}, runYes, false)
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		},
	}
	run.Flags().BoolVarP(&runYes, "yes", "y", false, "confirm prod")
	cmd.AddCommand(run)

	ps := &cobra.Command{Use: "pubsub", Short: "Pub/Sub"}
	var peekYes bool
	peek := &cobra.Command{
		Use: "peek <alias> <subscription>", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			b, err := c.GCPCall(args[0], "pubsub.peek", map[string]any{"subscription": args[1]}, peekYes, false)
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		},
	}
	peek.Flags().BoolVarP(&peekYes, "yes", "y", false, "confirm prod")
	ps.AddCommand(peek)
	cmd.AddCommand(ps)

	auth := &cobra.Command{Use: "auth", Short: "GCP auth"}
	auth.AddCommand(&cobra.Command{
		Use: "check", Short: "Verify ADC and reachable aliases",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			b, err := c.GCPAuth()
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		},
	})
	cmd.AddCommand(auth)
	return cmd
}

func newE2ECmd() *cobra.Command {
	cmd := &cobra.Command{Use: "e2e", Short: "Isolated E2E runs"}
	cmd.AddCommand(&cobra.Command{
		Use: "build", Short: "Build the E2E image",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			pr, err := cwdProfile()
			if err != nil {
				return err
			}
			b, err := c.E2EBuild(pr.Name)
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		},
	})
	var grep, shard string
	run := &cobra.Command{
		Use: "run", Short: "Queue an E2E run",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			pr, err := cwdProfile()
			if err != nil {
				return err
			}
			b, err := c.E2ERun(pr.Name, map[string]any{"grep": grep, "shard": shard})
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		},
	}
	run.Flags().StringVar(&grep, "grep", "", "playwright grep")
	run.Flags().StringVar(&shard, "shard", "", "shard like 1/3")
	cmd.AddCommand(run)
	cmd.AddCommand(&cobra.Command{
		Use: "ls", Short: "List runs",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			project := ""
			if pr, err := cwdProfile(); err == nil {
				project = pr.Name
			}
			b, err := c.E2EList(project)
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		},
	})
	var follow bool
	logs := &cobra.Command{
		Use: "logs <runid>", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			for {
				s, err := c.E2ELogs(args[0])
				if err != nil {
					return err
				}
				fmt.Print(s)
				if !follow {
					return nil
				}
				st, _ := c.E2EGet(args[0])
				if strings.Contains(string(st), `"status":"passed"`) || strings.Contains(string(st), `"status":"failed"`) || strings.Contains(string(st), `"status":"cancelled"`) {
					return nil
				}
				time.Sleep(time.Second)
			}
		},
	}
	logs.Flags().BoolVarP(&follow, "follow", "f", false, "follow")
	cmd.AddCommand(logs)
	cmd.AddCommand(&cobra.Command{
		Use: "open <runid>", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			b, err := c.E2EGet(args[0])
			if err != nil {
				return err
			}
			var run struct {
				ArtifactPath string `json:"artifact_path"`
			}
			_ = json.Unmarshal(b, &run)
			html := filepath.Join(run.ArtifactPath, "playwright-report", "index.html")
			if _, err := os.Stat(html); err != nil {
				html = run.ArtifactPath
			}
			return exec.Command("open", html).Run()
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use: "cancel <runid>", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			return c.E2ECancel(args[0])
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use: "prune",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			return c.E2EPrune()
		},
	})
	_ = e2e.DetectRuntime()
	return cmd
}

func cwdProfile() (*profile.Profile, error) {
	wd, _ := os.Getwd()
	return profile.Load(wd)
}

func newSecretsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "secrets", Short: "Secrets broker"}
	cmd.AddCommand(&cobra.Command{
		Use: "set <name>", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := readSecretValue(os.Stdin, term.IsTerminal(int(syscall.Stdin)))
			if err != nil {
				return err
			}
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			return c.SecretsSet(args[0], string(b))
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use: "ls",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			b, err := c.SecretsList()
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use: "rm <name>", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			return c.SecretsRm(args[0])
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use: "check [project]", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			project := ""
			if len(args) == 1 {
				project = args[0]
			} else if pr, err := cwdProfile(); err == nil {
				project = pr.Name
			}
			b, err := c.SecretsCheck(project)
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use: "lock",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			return c.SecretsLock()
		},
	})
	return cmd
}

func newAlertsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "alerts", Short: "Notifications"}
	var asJSON bool
	ls := &cobra.Command{
		Use: "ls",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			b, err := c.Alerts()
			if err != nil {
				return err
			}
			if asJSON {
				fmt.Println(string(b))
				return nil
			}
			var rows []map[string]any
			_ = json.Unmarshal(b, &rows)
			var table [][]string
			for _, r := range rows {
				table = append(table, []string{
					fmt.Sprint(r["ts"]), fmt.Sprint(r["type"]), fmt.Sprint(r["target"]), fmt.Sprint(r["project"]),
				})
			}
			return printTable(os.Stdout, []string{"TIME", "TYPE", "TARGET", "PROJECT"}, table)
		},
	}
	ls.Flags().BoolVar(&asJSON, "json", false, "raw JSON")
	cmd.AddCommand(ls)
	cmd.AddCommand(&cobra.Command{
		Use: "test",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			return c.AlertsTest()
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use: "mute <duration>", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mustDaemon()
			if err != nil {
				return err
			}
			return c.AlertsMute(args[0])
		},
	})
	return cmd
}

func newGCPUICmd() *cobra.Command {
	return &cobra.Command{Use: "gcp-ui", Hidden: true, RunE: func(cmd *cobra.Command, args []string) error {
		withCaller("tui")
		return ui.RunGCP()
	}}
}

func newE2EUICmd() *cobra.Command {
	return &cobra.Command{Use: "e2e-ui", Hidden: true, RunE: func(cmd *cobra.Command, args []string) error {
		withCaller("tui")
		return ui.RunE2E()
	}}
}

func readSecretValue(in io.Reader, isTTY bool) ([]byte, error) {
	if isTTY {
		fmt.Fprint(os.Stderr, "value: ")
		b, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Fprintln(os.Stderr)
		return b, err
	}
	b, err := io.ReadAll(in)
	if err != nil {
		return nil, err
	}
	return []byte(strings.TrimSpace(string(b))), nil
}

func newAlertsUICmd() *cobra.Command {
	return &cobra.Command{Use: "alerts-ui", Hidden: true, RunE: func(cmd *cobra.Command, args []string) error {
		withCaller("tui")
		return ui.RunAlerts()
	}}
}
