package registry

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rushikesh/devhub/internal/tmux"

	_ "modernc.org/sqlite"
)

type Kind string

const (
	KindActive Kind = "active"
	KindShadow Kind = "shadow"
)

type Status string

const (
	StatusRunning Status = "running"
	StatusExited  Status = "exited"
)

type Session struct {
	ID         int64
	Project    string
	Name       string
	Kind       Kind
	Command    string
	Cwd        string
	TmuxTarget string
	PanePID    int
	CreatedAt  time.Time
	Status     Status
	ExitCode   *int
}

type DB struct {
	sql *sql.DB
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	sqldb, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqldb.SetMaxOpenConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA foreign_keys=ON;",
	} {
		if _, err := sqldb.Exec(pragma); err != nil {
			_ = sqldb.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}
	db := &DB{sql: sqldb}
	if err := db.migrate(); err != nil {
		_ = sqldb.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) Close() error {
	if db.sql == nil {
		return nil
	}
	return db.sql.Close()
}

func (db *DB) migrate() error {
	_, err := db.sql.Exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY
);
CREATE TABLE IF NOT EXISTS sessions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project TEXT NOT NULL,
  name TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('active','shadow')),
  command TEXT,
  cwd TEXT,
  tmux_target TEXT,
  pane_pid INTEGER,
  created_at DATETIME NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('running','exited')),
  exit_code INTEGER,
  UNIQUE(project, name)
);
INSERT OR IGNORE INTO schema_migrations(version) VALUES (1);
`)
	return err
}

func (db *DB) Insert(s Session) (int64, error) {
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now()
	}
	if s.Status == "" {
		s.Status = StatusRunning
	}
	res, err := db.sql.Exec(`
INSERT INTO sessions (project, name, kind, command, cwd, tmux_target, pane_pid, created_at, status, exit_code)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.Project, s.Name, string(s.Kind), s.Command, s.Cwd, s.TmuxTarget, s.PanePID,
		s.CreatedAt.UTC().Format(time.RFC3339), string(s.Status), s.ExitCode,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (db *DB) Update(s Session) error {
	_, err := db.sql.Exec(`
UPDATE sessions SET kind=?, command=?, cwd=?, tmux_target=?, pane_pid=?, status=?, exit_code=?, name=?, project=?
WHERE id=?`,
		string(s.Kind), s.Command, s.Cwd, s.TmuxTarget, s.PanePID, string(s.Status), s.ExitCode, s.Name, s.Project, s.ID,
	)
	return err
}

func (db *DB) Delete(id int64) error {
	_, err := db.sql.Exec(`DELETE FROM sessions WHERE id=?`, id)
	return err
}

func (db *DB) Get(id int64) (Session, error) {
	return scanSession(db.sql.QueryRow(`SELECT id, project, name, kind, command, cwd, tmux_target, pane_pid, created_at, status, exit_code FROM sessions WHERE id=?`, id))
}

func (db *DB) List(includeExited bool) ([]Session, error) {
	q := `SELECT id, project, name, kind, command, cwd, tmux_target, pane_pid, created_at, status, exit_code FROM sessions`
	if !includeExited {
		q += ` WHERE status='running'`
	}
	q += ` ORDER BY project, kind, name`
	rows, err := db.sql.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (db *DB) ListByKind(kind Kind, includeExited bool) ([]Session, error) {
	all, err := db.List(includeExited)
	if err != nil {
		return nil, err
	}
	var out []Session
	for _, s := range all {
		if s.Kind == kind {
			out = append(out, s)
		}
	}
	return out, nil
}

func (db *DB) FindByName(name string) ([]Session, error) {
	rows, err := db.sql.Query(`SELECT id, project, name, kind, command, cwd, tmux_target, pane_pid, created_at, status, exit_code FROM sessions WHERE name=?`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (db *DB) GetByProjectName(project, name string) (Session, error) {
	return scanSession(db.sql.QueryRow(`SELECT id, project, name, kind, command, cwd, tmux_target, pane_pid, created_at, status, exit_code FROM sessions WHERE project=? AND name=?`, project, name))
}

func (db *DB) NamesInProject(project string, kind Kind) ([]string, error) {
	rows, err := db.sql.Query(`SELECT name FROM sessions WHERE project=? AND kind=?`, project, string(kind))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSession(row rowScanner) (Session, error) {
	var s Session
	var kind, status, created, command, cwd, target sql.NullString
	var pid sql.NullInt64
	var exit sql.NullInt64
	err := row.Scan(&s.ID, &s.Project, &s.Name, &kind, &command, &cwd, &target, &pid, &created, &status, &exit)
	if err != nil {
		return Session{}, err
	}
	s.Kind = Kind(kind.String)
	s.Status = Status(status.String)
	s.Command = command.String
	s.Cwd = cwd.String
	s.TmuxTarget = target.String
	s.PanePID = int(pid.Int64)
	if created.Valid {
		if t, err := time.Parse(time.RFC3339, created.String); err == nil {
			s.CreatedAt = t
		} else if t, err := time.Parse("2006-01-02 15:04:05", created.String); err == nil {
			s.CreatedAt = t
		}
	}
	if exit.Valid {
		n := int(exit.Int64)
		s.ExitCode = &n
	}
	return s, nil
}

func matchPane(s Session, panes []tmux.Pane) (tmux.Pane, bool) {
	for _, p := range panes {
		if s.TmuxTarget != "" && p.Target == s.TmuxTarget {
			return p, true
		}
		if p.Session == expectedSession(s) && p.Window == s.Name {
			return p, true
		}
	}
	return tmux.Pane{}, false
}

func expectedSession(s Session) string {
	if s.Kind == KindShadow {
		return tmux.ShadowSession(s.Project)
	}
	return tmux.ProjectSession(s.Project)
}

// Reconcile marks registry rows exited when their tmux pane is gone or dead.
func (db *DB) Reconcile(panes []tmux.Pane) error {
	sessions, err := db.List(true)
	if err != nil {
		return err
	}
	for _, s := range sessions {
		if s.Status == StatusExited {
			continue
		}
		p, ok := matchPane(s, panes)
		if !ok {
			s.Status = StatusExited
			s.PanePID = 0
			if err := db.Update(s); err != nil {
				return err
			}
			continue
		}
		s.TmuxTarget = p.Target
		s.PanePID = p.PID
		if p.Dead {
			s.Status = StatusExited
			if p.DeadStatus >= 0 {
				code := p.DeadStatus
				s.ExitCode = &code
			}
			s.PanePID = p.PID
		} else {
			s.Status = StatusRunning
		}
		if err := db.Update(s); err != nil {
			return err
		}
	}
	return nil
}
