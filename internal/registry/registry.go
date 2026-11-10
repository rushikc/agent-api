package registry

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	StatusStopped Status = "stopped"
)

type Session struct {
	ID           int64
	Project      string
	Name         string
	Kind         Kind
	Command      string
	Cwd          string
	TmuxTarget   string
	PanePID      int
	CreatedAt    time.Time
	Status       Status
	ExitCode     *int
	RestartCount int
	Port         int
	ArgvJSON     string
	Restart      string
}

type AuditEntry struct {
	ID         int64
	TS         time.Time
	Caller     string
	Action     string
	Target     string
	Result     string
	Project    string
	Task       string
	ParamsHash string
	DurationMS int64
	ResultSize int64
}

type E2ERun struct {
	ID           string    `json:"id"`
	Project      string    `json:"project"`
	GitSHA       string    `json:"git_sha"`
	Dirty        bool      `json:"dirty"`
	Status       string    `json:"status"`
	DurationMS   int64     `json:"duration_ms"`
	ExitCode     *int      `json:"exit_code,omitempty"`
	ArtifactPath string    `json:"artifact_path"`
	Summary      string    `json:"summary"`
	SummaryText  string    `json:"summary_text,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type EventRow struct {
	ID      int64     `json:"id"`
	TS      time.Time `json:"ts"`
	Type    string    `json:"type"`
	Target  string    `json:"target"`
	Project string    `json:"project"`
	Payload string    `json:"payload"`
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
	sqldb.SetMaxOpenConns(8)
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
	if err != nil {
		return err
	}
	var v int
	_ = db.sql.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v)
	if v < 2 {
		for _, stmt := range []string{
			`CREATE TABLE IF NOT EXISTS audit (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts DATETIME NOT NULL,
  caller TEXT NOT NULL,
  action TEXT NOT NULL,
  target TEXT,
  result TEXT
);`,
			`ALTER TABLE sessions ADD COLUMN restart_count INTEGER NOT NULL DEFAULT 0;`,
			`ALTER TABLE sessions ADD COLUMN port INTEGER;`,
			`INSERT OR IGNORE INTO schema_migrations(version) VALUES (2);`,
		} {
			if _, err := db.sql.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
	}
	_ = db.sql.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v)
	if v < 3 {
		for _, stmt := range []string{
			`ALTER TABLE audit ADD COLUMN project TEXT;`,
			`ALTER TABLE audit ADD COLUMN task TEXT;`,
			`ALTER TABLE audit ADD COLUMN params_hash TEXT;`,
			`ALTER TABLE audit ADD COLUMN duration_ms INTEGER;`,
			`ALTER TABLE audit ADD COLUMN result_size INTEGER;`,
			`CREATE TABLE IF NOT EXISTS e2e_runs (
  id TEXT PRIMARY KEY,
  project TEXT NOT NULL,
  git_sha TEXT,
  dirty INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  duration_ms INTEGER,
  exit_code INTEGER,
  artifact_path TEXT,
  summary TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);`,
			`CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts DATETIME NOT NULL,
  type TEXT NOT NULL,
  target TEXT,
  project TEXT,
  payload TEXT
);`,
			`INSERT OR IGNORE INTO schema_migrations(version) VALUES (3);`,
		} {
			if _, err := db.sql.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
	}
	_ = db.sql.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v)
	if v < 4 {
		for _, stmt := range []string{
			`CREATE TABLE sessions_v4 (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project TEXT NOT NULL,
  name TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('active','shadow')),
  command TEXT,
  cwd TEXT,
  tmux_target TEXT,
  pane_pid INTEGER,
  created_at DATETIME NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('running','exited','stopped')),
  exit_code INTEGER,
  restart_count INTEGER NOT NULL DEFAULT 0,
  port INTEGER,
  UNIQUE(project, name)
);`,
			`INSERT INTO sessions_v4 (id, project, name, kind, command, cwd, tmux_target, pane_pid, created_at, status, exit_code, restart_count, port)
SELECT id, project, name, kind, command, cwd, tmux_target, pane_pid, created_at, status, exit_code, restart_count, port FROM sessions;`,
			`DROP TABLE sessions;`,
			`ALTER TABLE sessions_v4 RENAME TO sessions;`,
			`INSERT OR IGNORE INTO schema_migrations(version) VALUES (4);`,
		} {
			if _, err := db.sql.Exec(stmt); err != nil {
				return err
			}
		}
	}
	_ = db.sql.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v)
	if v < 5 {
		for _, stmt := range []string{
			`CREATE TABLE IF NOT EXISTS secret_names (
  name TEXT PRIMARY KEY
);`,
			`INSERT OR IGNORE INTO schema_migrations(version) VALUES (5);`,
		} {
			if _, err := db.sql.Exec(stmt); err != nil {
				return err
			}
		}
	}
	_ = db.sql.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v)
	if v < 6 {
		for _, stmt := range []string{
			`ALTER TABLE sessions ADD COLUMN argv TEXT;`,
			`ALTER TABLE sessions ADD COLUMN restart TEXT;`,
			`INSERT OR IGNORE INTO schema_migrations(version) VALUES (6);`,
		} {
			if _, err := db.sql.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
	}
	return nil
}

func (db *DB) PutSecretName(name string) error {
	_, err := db.sql.Exec(`INSERT OR REPLACE INTO secret_names(name) VALUES (?)`, name)
	return err
}

func (db *DB) DeleteSecretName(name string) error {
	_, err := db.sql.Exec(`DELETE FROM secret_names WHERE name=?`, name)
	return err
}

func (db *DB) ListSecretNames() ([]string, error) {
	rows, err := db.sql.Query(`SELECT name FROM secret_names ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (db *DB) Insert(s Session) (int64, error) {
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now()
	}
	if s.Status == "" {
		s.Status = StatusRunning
	}
	res, err := db.sql.Exec(`
INSERT INTO sessions (project, name, kind, command, cwd, tmux_target, pane_pid, created_at, status, exit_code, restart_count, port, argv, restart)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.Project, s.Name, string(s.Kind), s.Command, s.Cwd, s.TmuxTarget, s.PanePID,
		s.CreatedAt.UTC().Format(time.RFC3339), string(s.Status), s.ExitCode, s.RestartCount, nullPort(s.Port), s.ArgvJSON, s.Restart,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (db *DB) Update(s Session) error {
	_, err := db.sql.Exec(`
UPDATE sessions SET kind=?, command=?, cwd=?, tmux_target=?, pane_pid=?, status=?, exit_code=?, name=?, project=?, restart_count=?, port=?, argv=?, restart=?
WHERE id=?`,
		string(s.Kind), s.Command, s.Cwd, s.TmuxTarget, s.PanePID, string(s.Status), s.ExitCode, s.Name, s.Project, s.RestartCount, nullPort(s.Port), s.ArgvJSON, s.Restart, s.ID,
	)
	return err
}

func nullPort(p int) any {
	if p == 0 {
		return nil
	}
	return p
}

func (db *DB) Audit(caller, action, target, result string) error {
	return db.AuditCall(caller, action, target, result, "", "", "", 0, 0)
}

func (db *DB) AuditCall(caller, action, target, result, project, task, paramsHash string, durationMS, resultSize int64) error {
	if caller == "" {
		caller = "cli"
	}
	_, err := db.sql.Exec(`INSERT INTO audit (ts, caller, action, target, result, project, task, params_hash, duration_ms, result_size) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		time.Now().UTC().Format(time.RFC3339), caller, action, target, result, project, task, paramsHash, durationMS, resultSize)
	return err
}

func (db *DB) ListAudit(limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.sql.Query(`SELECT id, ts, caller, action, target, result, COALESCE(project,''), COALESCE(task,''), COALESCE(params_hash,''), COALESCE(duration_ms,0), COALESCE(result_size,0) FROM audit ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var ts string
		if err := rows.Scan(&e.ID, &ts, &e.Caller, &e.Action, &e.Target, &e.Result, &e.Project, &e.Task, &e.ParamsHash, &e.DurationMS, &e.ResultSize); err != nil {
			return nil, err
		}
		e.TS, _ = time.Parse(time.RFC3339, ts)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (db *DB) InsertE2ERun(r E2ERun) error {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = r.CreatedAt
	}
	dirty := 0
	if r.Dirty {
		dirty = 1
	}
	_, err := db.sql.Exec(`INSERT INTO e2e_runs (id, project, git_sha, dirty, status, duration_ms, exit_code, artifact_path, summary, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.Project, r.GitSHA, dirty, r.Status, r.DurationMS, r.ExitCode, r.ArtifactPath, r.Summary,
		r.CreatedAt.Format(time.RFC3339), r.UpdatedAt.Format(time.RFC3339))
	return err
}

func (db *DB) UpdateE2ERun(r E2ERun) error {
	r.UpdatedAt = time.Now().UTC()
	dirty := 0
	if r.Dirty {
		dirty = 1
	}
	_, err := db.sql.Exec(`UPDATE e2e_runs SET project=?, git_sha=?, dirty=?, status=?, duration_ms=?, exit_code=?, artifact_path=?, summary=?, updated_at=? WHERE id=?`,
		r.Project, r.GitSHA, dirty, r.Status, r.DurationMS, r.ExitCode, r.ArtifactPath, r.Summary, r.UpdatedAt.Format(time.RFC3339), r.ID)
	return err
}

func (db *DB) GetE2ERun(id string) (E2ERun, error) {
	return scanE2E(db.sql.QueryRow(`SELECT id, project, git_sha, dirty, status, duration_ms, exit_code, artifact_path, summary, created_at, updated_at FROM e2e_runs WHERE id=?`, id))
}

func (db *DB) ListE2ERuns(project string, limit int) ([]E2ERun, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT id, project, git_sha, dirty, status, duration_ms, exit_code, artifact_path, summary, created_at, updated_at FROM e2e_runs`
	var args []any
	if project != "" {
		q += ` WHERE project=?`
		args = append(args, project)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.sql.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []E2ERun
	for rows.Next() {
		r, err := scanE2E(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanE2E(row rowScanner) (E2ERun, error) {
	var r E2ERun
	var dirty int
	var exit sql.NullInt64
	var created, updated string
	err := row.Scan(&r.ID, &r.Project, &r.GitSHA, &dirty, &r.Status, &r.DurationMS, &exit, &r.ArtifactPath, &r.Summary, &created, &updated)
	if err != nil {
		return E2ERun{}, err
	}
	r.Dirty = dirty != 0
	if exit.Valid {
		n := int(exit.Int64)
		r.ExitCode = &n
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339, created)
	r.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return r, nil
}

func (db *DB) InsertEvent(e EventRow) error {
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	_, err := db.sql.Exec(`INSERT INTO events (ts, type, target, project, payload) VALUES (?,?,?,?,?)`,
		e.TS.Format(time.RFC3339), e.Type, e.Target, e.Project, e.Payload)
	return err
}

func (db *DB) ListEvents(limit int) ([]EventRow, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.sql.Query(`SELECT id, ts, type, target, project, payload FROM events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EventRow
	for rows.Next() {
		var e EventRow
		var ts string
		if err := rows.Scan(&e.ID, &ts, &e.Type, &e.Target, &e.Project, &e.Payload); err != nil {
			return nil, err
		}
		e.TS, _ = time.Parse(time.RFC3339, ts)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (db *DB) Delete(id int64) error {
	_, err := db.sql.Exec(`DELETE FROM sessions WHERE id=?`, id)
	return err
}

func (db *DB) Get(id int64) (Session, error) {
	return scanSession(db.sql.QueryRow(sessionSelect+` WHERE id=?`, id))
}

const sessionSelect = `SELECT id, project, name, kind, command, cwd, tmux_target, pane_pid, created_at, status, exit_code, COALESCE(restart_count,0), COALESCE(port,0), COALESCE(argv,''), COALESCE(restart,'') FROM sessions`

func (db *DB) List(includeExited bool) ([]Session, error) {
	q := sessionSelect
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
	rows, err := db.sql.Query(sessionSelect+` WHERE name=?`, name)
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
	return scanSession(db.sql.QueryRow(sessionSelect+` WHERE project=? AND name=?`, project, name))
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
	var restarts, port sql.NullInt64
	var argv, restart sql.NullString
	err := row.Scan(&s.ID, &s.Project, &s.Name, &kind, &command, &cwd, &target, &pid, &created, &status, &exit, &restarts, &port, &argv, &restart)
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
	s.RestartCount = int(restarts.Int64)
	s.Port = int(port.Int64)
	s.ArgvJSON = argv.String
	s.Restart = restart.String
	return s, nil
}

func matchPane(s Session, panes []tmux.Pane) (tmux.Pane, bool) {
	wantSess := expectedSession(s)
	wantID := windowIDFromTarget(s.TmuxTarget)
	for _, p := range panes {
		if s.TmuxTarget != "" && (p.Target == s.TmuxTarget || p.Session+":"+p.WindowID == s.TmuxTarget) {
			return p, true
		}
		if wantID != "" && p.Session == wantSess && (p.WindowID == wantID || p.WindowID == "@"+strings.TrimPrefix(wantID, "@")) {
			return p, true
		}
		if p.Session == wantSess && p.Window == s.Name {
			return p, true
		}
	}
	return tmux.Pane{}, false
}

func windowIDFromTarget(target string) string {
	_, rest, ok := strings.Cut(target, ":")
	if !ok {
		return ""
	}
	if strings.HasPrefix(rest, "@") {
		return rest
	}
	return ""
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
		if s.Status == StatusExited || s.Status == StatusStopped {
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
