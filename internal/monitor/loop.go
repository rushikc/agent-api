package monitor

import (
	"os"
	"time"

	"github.com/rushikesh/devhub/internal/registry"
	"github.com/rushikesh/devhub/internal/tmux"
	"golang.org/x/sys/unix"
)

func TryLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func HasDevhubSessions(client tmux.Client) (bool, error) {
	ss, err := client.ListSessions()
	if err != nil {
		return false, err
	}
	for _, s := range ss {
		if tmux.IsDevhubSession(s.Name) {
			return true, nil
		}
	}
	return false, nil
}

func RunLoop(client tmux.Client, db *registry.DB, statusPath string, interval time.Duration, stop <-chan struct{}) error {
	sampler := NewSampler(nil)
	tick := time.NewTicker(interval)
	defer tick.Stop()

	sampleOnce := func() error {
		alive, err := HasDevhubSessions(client)
		if err != nil {
			return err
		}
		if !alive {
			return errNoSessions
		}
		panes, err := client.ListPanes()
		if err != nil {
			return err
		}
		if err := db.Reconcile(panes); err != nil {
			return err
		}
		sessions, err := db.List(true)
		if err != nil {
			return err
		}
		snap := BuildSnapshot(sessions, sampler)
		return WriteSnapshot(statusPath, snap)
	}

	if err := sampleOnce(); err != nil {
		if err == errNoSessions {
			return nil
		}
		return err
	}

	for {
		select {
		case <-stop:
			return nil
		case <-tick.C:
			if err := sampleOnce(); err != nil {
				if err == errNoSessions {
					return nil
				}
				return err
			}
		}
	}
}

var errNoSessions = os.ErrNotExist
