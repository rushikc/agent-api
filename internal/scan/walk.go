package scan

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/rushikesh/devhub/internal/config"
	"golang.org/x/sys/unix"
)

type item struct {
	path  string
	depth int
	dev   uint64
}

type walker struct {
	opts    Options
	mu      sync.Mutex
	q       []item
	active  int
	cond    *sync.Cond
	seen    map[string]struct{}
	repos   []Repo
	dirs    int
	denied  int
	privacy bool
	partial bool
}

func Walk(ctx context.Context, opts Options) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if opts.Depth <= 0 {
		opts.Depth = 6
	}
	if opts.Workers <= 0 {
		opts.Workers = 16
	}
	if opts.Home == "" {
		opts.Home, _ = os.UserHomeDir()
	}
	start := time.Now()
	w := &walker{
		opts: opts,
		seen: map[string]struct{}{},
	}
	w.cond = sync.NewCond(&w.mu)

	for _, root := range opts.Roots {
		expanded, err := config.ExpandPath(root)
		if err != nil {
			continue
		}
		expanded = filepath.Clean(expanded)
		st, err := os.Lstat(expanded)
		if err != nil {
			if isDenied(err) {
				w.denied++
				if isPrivacyDir(expanded, opts.Home) {
					w.privacy = true
				}
			}
			continue
		}
		if st.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if !st.IsDir() {
			continue
		}
		dev, err := deviceID(expanded)
		if err != nil {
			continue
		}
		w.enqueue(item{path: expanded, depth: 0, dev: dev})
	}

	go func() {
		<-ctx.Done()
		w.mu.Lock()
		w.partial = true
		w.cond.Broadcast()
		w.mu.Unlock()
	}()

	var wg sync.WaitGroup
	n := opts.Workers
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			w.worker(ctx)
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	if opts.Progress != nil {
		tick := time.NewTicker(80 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				opts.Progress(w.snapshot(time.Since(start)))
				goto finished
			case <-tick.C:
				opts.Progress(w.snapshot(time.Since(start)))
			}
		}
	}
	<-done
finished:
	w.mu.Lock()
	defer w.mu.Unlock()
	if ctx.Err() != nil {
		w.partial = true
	}
	res := &Result{
		Repos:          append([]Repo(nil), w.repos...),
		Dirs:           w.dirs,
		Denied:         w.denied,
		PrivacyDenied:  w.privacy,
		Roots:          append([]string(nil), opts.Roots...),
		Depth:          opts.Depth,
		Nested:         opts.Nested,
		IncludeVolumes: opts.IncludeVolumes,
		Partial:        w.partial,
	}
	res.DurationMS, res.Duration = setElapsed(time.Since(start))
	Annotate(res, opts.Projects)
	return res, nil
}

func Annotate(res *Result, projects map[string]string) {
	if res == nil || projects == nil {
		return
	}
	cfg := &config.Config{Projects: projects}
	for i := range res.Repos {
		if name, ok := cfg.ProjectNameForPath(res.Repos[i].Path); ok {
			res.Repos[i].Registered = true
			res.Repos[i].RegisteredName = name
		}
	}
}

func (w *walker) snapshot(elapsed time.Duration) Progress {
	w.mu.Lock()
	defer w.mu.Unlock()
	cur := ""
	if len(w.q) > 0 {
		cur = w.q[0].path
	}
	ms, human := setElapsed(elapsed)
	return Progress{
		Dirs:      w.dirs,
		Repos:     len(w.repos),
		Denied:    w.denied,
		Current:   cur,
		ElapsedMS: ms,
		Elapsed:   human,
		Privacy:   w.privacy,
	}
}

func (w *walker) enqueue(it item) {
	clean := filepath.Clean(it.path)
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.seen[clean]; ok {
		return
	}
	w.seen[clean] = struct{}{}
	w.q = append(w.q, it)
	w.cond.Signal()
}

func (w *walker) worker(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			w.mu.Lock()
			w.partial = true
			w.cond.Broadcast()
			w.mu.Unlock()
			return
		}
		it, ok := w.take()
		if !ok {
			return
		}
		w.visit(ctx, it)
		w.mu.Lock()
		w.active--
		w.cond.Broadcast()
		w.mu.Unlock()
	}
}

func (w *walker) take() (item, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for len(w.q) == 0 && w.active > 0 && !w.partial {
		w.cond.Wait()
	}
	if w.partial || len(w.q) == 0 {
		return item{}, false
	}
	it := w.q[0]
	w.q = w.q[1:]
	w.active++
	return it, true
}

func (w *walker) visit(ctx context.Context, it item) {
	if ctx.Err() != nil {
		return
	}
	if skipPath(it.path, skipOpts{exclude: w.opts.Exclude, home: w.opts.Home, includeHidden: w.opts.IncludeHidden, allow: w.opts.AllowRoots}) {
		return
	}
	entries, err := os.ReadDir(it.path)
	w.mu.Lock()
	w.dirs++
	w.mu.Unlock()
	if err != nil {
		if isDenied(err) {
			w.mu.Lock()
			w.denied++
			if isPrivacyDir(it.path, w.opts.Home) {
				w.privacy = true
			}
			w.mu.Unlock()
		}
		return
	}
	canon := config.CanonPath(it.path)
	foundGit := isGitRepo(it.path, entries)
	if foundGit {
		repo := inspectRepo(it.path)
		repo.Path = canon
		if repo.Name == "" || repo.Name == "." {
			repo.Name = filepath.Base(canon)
		}
		w.mu.Lock()
		w.repos = append(w.repos, repo)
		w.mu.Unlock()
		if !w.opts.Nested {
			return
		}
	}
	if it.depth >= w.opts.Depth {
		return
	}
	for _, e := range entries {
		if ctx.Err() != nil {
			return
		}
		name := e.Name()
		if name == "." || name == ".." || name == ".git" {
			continue
		}
		child := filepath.Join(it.path, name)
		typ := e.Type()
		if typ&os.ModeSymlink != 0 {
			continue
		}
		if !e.IsDir() {
			continue
		}
		if skipPath(child, skipOpts{exclude: w.opts.Exclude, home: w.opts.Home, includeHidden: w.opts.IncludeHidden, allow: w.opts.AllowRoots}) {
			continue
		}
		dev := it.dev
		if d, err := deviceID(child); err == nil {
			if d != it.dev && !w.opts.IncludeVolumes {
				continue
			}
			dev = d
		}
		w.enqueue(item{path: child, depth: it.depth + 1, dev: dev})
	}
}

func deviceID(path string) (uint64, error) {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return 0, err
	}
	return uint64(st.Dev), nil
}

func isDenied(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrPermission) || errors.Is(err, fs.ErrPermission) {
		return true
	}
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	return errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES)
}

func PrivacyHint() string {
	return "macOS blocked a protected folder. Grant the terminal Full Disk Access or Files and Folders in System Settings > Privacy & Security."
}
