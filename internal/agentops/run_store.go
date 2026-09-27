package agentops

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// runStore persists agent runs so their history survives a client restart. It
// is optional: a nil *runStore is a no-op, which keeps in-memory-only mode
// (tests, and a Service without a store path) free of branches.
type runStore struct {
	db *sql.DB
}

// openRunStore opens, or creates, the run database. An empty path disables
// persistence and returns a nil store.
func openRunStore(path string) (*runStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, fmt.Errorf("open agent run store: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS agent_runs (
		id          TEXT PRIMARY KEY,
		workspace   TEXT NOT NULL DEFAULT '',
		client      TEXT NOT NULL DEFAULT '',
		agent       TEXT NOT NULL DEFAULT '',
		model       TEXT NOT NULL DEFAULT '',
		thinking    TEXT NOT NULL DEFAULT '',
		task        TEXT NOT NULL DEFAULT '',
		status      TEXT NOT NULL DEFAULT '',
		started_at  INTEGER NOT NULL DEFAULT 0,
		finished_at INTEGER NOT NULL DEFAULT 0,
		elapsed_ms  INTEGER NOT NULL DEFAULT 0,
		output      TEXT NOT NULL DEFAULT '',
		error       TEXT NOT NULL DEFAULT '',
		event_count INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_agent_runs_started ON agent_runs(started_at DESC);`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init agent run store: %w", err)
	}
	// A run still marked queued or running belongs to a previous process: the
	// binary is gone, so the run can never finish.
	if _, err := db.Exec(`UPDATE agent_runs SET status = ? WHERE status IN (?, ?)`,
		string(RunInterrupted), string(RunQueued), string(RunRunning)); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("recover agent runs: %w", err)
	}
	return &runStore{db: db}, nil
}

func (s *runStore) Close() error {
	if s == nil {
		return nil
	}
	return s.db.Close()
}

// upsert writes the whole run row. Runs are small and rewritten on each
// status change, so a full replace keeps the row and the snapshot identical
// without merge logic.
func (s *runStore) upsert(snap *AgentRun) error {
	if s == nil || snap == nil {
		return nil
	}
	_, err := s.db.Exec(`INSERT OR REPLACE INTO agent_runs
		(id, workspace, client, agent, model, thinking, task, status, started_at, finished_at, elapsed_ms, output, error, event_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.ID, snap.Workspace, snap.Client, snap.Agent, snap.Model, snap.Thinking, snap.Task,
		string(snap.Status), unixMilli(snap.StartedAt), unixMilli(snap.FinishedAt), snap.ElapsedMS,
		snap.Output, snap.Error, snap.EventCount)
	return err
}

// get returns one stored run, or nil when the id is unknown.
func (s *runStore) get(id string) (*AgentRun, error) {
	if s == nil {
		return nil, nil
	}
	row := s.db.QueryRow(`SELECT `+runColumns+` FROM agent_runs WHERE id = ?`, id)
	snap, err := scanRun(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return snap, nil
}

// all returns every stored run, newest first.
func (s *runStore) all() ([]*AgentRun, error) {
	if s == nil {
		return nil, nil
	}
	rows, err := s.db.Query(`SELECT ` + runColumns + ` FROM agent_runs ORDER BY started_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AgentRun
	for rows.Next() {
		snap, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, snap)
	}
	return out, rows.Err()
}

const runColumns = `id, workspace, client, agent, model, thinking, task, status,
	started_at, finished_at, elapsed_ms, output, error, event_count`

// rowScanner is the common surface of *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanRun(row rowScanner) (*AgentRun, error) {
	var (
		snap       AgentRun
		status     string
		startedAt  int64
		finishedAt int64
	)
	if err := row.Scan(
		&snap.ID, &snap.Workspace, &snap.Client, &snap.Agent, &snap.Model, &snap.Thinking,
		&snap.Task, &status, &startedAt, &finishedAt, &snap.ElapsedMS,
		&snap.Output, &snap.Error, &snap.EventCount,
	); err != nil {
		return nil, err
	}
	snap.Status = RunStatus(status)
	snap.StartedAt = timeFromMilli(startedAt)
	snap.FinishedAt = timeFromMilli(finishedAt)
	return &snap, nil
}

// unixMilli renders a timestamp for the store; the zero time stays zero.
func unixMilli(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// timeFromMilli is the inverse of unixMilli.
func timeFromMilli(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}
