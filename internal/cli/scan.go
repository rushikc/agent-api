package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rushikesh/devhub/internal/core"
	"github.com/rushikesh/devhub/internal/scan"
	"github.com/rushikesh/devhub/internal/ui"
	"github.com/spf13/cobra"
)

type scanFlags struct {
	all            bool
	includeVolumes bool
	jsonOut        bool
	noUI           bool
	refresh        bool
	nested         bool
	paths          []string
	depth          int
}

func newScanCmd() *cobra.Command {
	f := &scanFlags{}
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Find git repos and manage projects",
		RunE:  scanRun(f),
	}
	bindScanFlags(cmd, f)
	cmd.AddCommand(&cobra.Command{
		Use:   "projects",
		Short: "Scan for git repositories and pick projects",
		RunE:  scanRun(f),
	})
	return cmd
}

func bindScanFlags(cmd *cobra.Command, f *scanFlags) {
	fs := cmd.PersistentFlags()
	fs.BoolVar(&f.all, "all", false, "scan home (and volumes if enabled)")
	fs.BoolVar(&f.includeVolumes, "include-volumes", false, "include /Volumes")
	fs.BoolVar(&f.jsonOut, "json", false, "print JSON")
	fs.BoolVar(&f.noUI, "no-ui", false, "print a table only")
	fs.BoolVar(&f.refresh, "refresh", false, "ignore the scan cache")
	fs.BoolVar(&f.nested, "nested", false, "include nested git repos")
	fs.StringArrayVar(&f.paths, "path", nil, "extra scan root (repeatable)")
	fs.IntVar(&f.depth, "depth", 0, "max directory depth")
}

func scanRun(f *scanFlags) func(*cobra.Command, []string) error {
	return withServiceOpt(false, func(svc *core.Service, args []string) error {
		return runScan(svc, *f, false)
	})
}

func newScanUICmd() *cobra.Command {
	return &cobra.Command{
		Use:    "scan-ui",
		Hidden: true,
		RunE: withService(func(svc *core.Service, args []string) error {
			withCaller("tui")
			return runScan(svc, scanFlags{}, true)
		}),
	}
}

func runScan(svc *core.Service, f scanFlags, cacheOnly bool) error {
	cfg := svc.Cfg
	vols := f.includeVolumes || (cfg != nil && cfg.Scan.IncludeVolumes)
	roots := scan.ResolveRoots(cfg, f.all, f.paths, vols)
	depth := scan.ResolveDepth(cfg, f.all, f.depth)
	exclude := []string{}
	projects := map[string]string{}
	if cfg != nil {
		exclude = cfg.Scan.Exclude
		projects = cfg.Projects
	}

	var res *scan.Result
	var fromCache bool
	var cacheAge time.Duration
	if !f.refresh {
		if c, err := scan.LoadCache(cfg.ScanCachePath()); err == nil {
			if cacheOnly || scan.Fresh(c, roots, depth, f.nested, vols, scan.DefaultMaxAge) {
				res = scan.ResultFromCache(c)
				scan.Annotate(res, projects)
				fromCache = true
				cacheAge = scan.Age(c)
			}
		}
	}

	if res == nil {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		start := time.Now()
		res, err := scan.Walk(ctx, scan.Options{
			Roots:          roots,
			Depth:          depth,
			Nested:         f.nested,
			IncludeVolumes: vols,
			Exclude:        exclude,
			Projects:       projects,
			Progress: func(p scan.Progress) {
				fmt.Fprintf(os.Stderr, "\rscanning %d dirs  %d repos  %s   ", p.Dirs, p.Repos, time.Since(start).Truncate(time.Millisecond))
			},
		})
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return err
		}
		if cfg != nil {
			_ = scan.SaveCache(cfg.ScanCachePath(), scan.CacheFromResult(res))
		}
		if res.PrivacyDenied {
			fmt.Fprintln(os.Stderr, scan.PrivacyHint())
		}
		if res.Partial {
			fmt.Fprintln(os.Stderr, "scan cancelled; showing partial results")
		}
		cacheAge = 0
		fromCache = false
		return finishScan(svc, res, cacheAge, fromCache, f)
	}
	if fromCache {
		fmt.Fprintf(os.Stderr, "scan cache %s old\n", cacheAge.Truncate(time.Second))
	}
	return finishScan(svc, res, cacheAge, fromCache, f)
}

func finishScan(svc *core.Service, res *scan.Result, cacheAge time.Duration, fromCache bool, f scanFlags) error {
	if f.jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	if f.noUI {
		return printScanTable(res)
	}
	return ui.RunScanPicker(svc, res, cacheAge, fromCache)
}

func printScanTable(res *scan.Result) error {
	if res == nil || len(res.Repos) == 0 {
		fmt.Println("no git repositories found")
		return nil
	}
	var rows [][]string
	for _, r := range res.Repos {
		added := ""
		if r.Registered {
			added = r.RegisteredName
		}
		act := ""
		if !r.LastActivity.IsZero() {
			act = r.LastActivity.Format("2006-01-02")
		}
		rows = append(rows, []string{r.Name, r.Path, r.Branch, strings.Join(r.Stack, ","), added, act})
	}
	return printTable(os.Stdout, []string{"NAME", "PATH", "BRANCH", "STACK", "ADDED", "ACTIVITY"}, rows)
}
