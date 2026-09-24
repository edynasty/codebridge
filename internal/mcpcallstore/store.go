// Package mcpcallstore persists MCP tool-call records into an embedded
// SQLite database. The JSONL audit log remains the append-only source of
// truth; this store exists so the admin console can filter, page and
// aggregate call history without scanning the file.
package mcpcallstore

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Call is one persisted MCP tool invocation.
type Call struct {
	Time       time.Time `json:"time"`
	RequestID  string    `json:"request_id"`
	ActorID    string    `json:"actor_id"`
	Tool       string    `json:"tool"`
	DeviceID   string    `json:"device_id"`
	Workspace  string    `json:"workspace"`
	Success    bool      `json:"success"`
	DurationMS int64     `json:"duration_ms"`
	ErrorKind  string    `json:"error_kind,omitempty"`
}

// Filter selects calls for the admin console. Zero values mean "any".
type Filter struct {
	Tool      string
	ActorID   string
	DeviceID  string
	Workspace string
	Query     string // substring over tool/device/workspace/request_id/actor
	Success   *bool
	Since     time.Time
	Until     time.Time
	Limit     int
	Offset    int
}

// Stats summarizes the matched set.
type Stats struct {
	Total   int64 `json:"total"`
	Failed  int64 `json:"failed"`
	AvgMS   int64 `json:"avg_ms"`
	TotalMS int64 `json:"total_ms"`
}

type Store struct {
	db *sql.DB
}

// Open creates or opens the database at path and ensures the schema.
func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil // disabled
	}
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, fmt.Errorf("open mcp call store: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS mcp_calls (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		time         TEXT NOT NULL,
		request_id   TEXT NOT NULL DEFAULT '',
		actor_id     TEXT NOT NULL DEFAULT '',
		tool         TEXT NOT NULL DEFAULT '',
		device_id    TEXT NOT NULL DEFAULT '',
		workspace    TEXT NOT NULL DEFAULT '',
		success      INTEGER NOT NULL DEFAULT 0,
		duration_ms  INTEGER NOT NULL DEFAULT 0,
		error_kind   TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_mcp_calls_time ON mcp_calls(time DESC);
	CREATE INDEX IF NOT EXISTS idx_mcp_calls_tool ON mcp_calls(tool);
	CREATE INDEX IF NOT EXISTS idx_mcp_calls_actor ON mcp_calls(actor_id);
	CREATE INDEX IF NOT EXISTS idx_mcp_calls_device ON mcp_calls(device_id);`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init mcp call store: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	return s.db.Close()
}

// Insert records one call. The logger never fails the request on store
// errors; the caller decides whether to log them.
func (s *Store) Insert(call Call) error {
	if s == nil {
		return nil
	}
	_, err := s.db.Exec(`INSERT INTO mcp_calls (time, request_id, actor_id, tool, device_id, workspace, success, duration_ms, error_kind)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		call.Time.UTC().Format(time.RFC3339Nano), call.RequestID, call.ActorID, call.Tool,
		call.DeviceID, call.Workspace, boolToInt(call.Success), call.DurationMS, call.ErrorKind)
	return err
}

// List returns calls matching the filter plus aggregate stats.
func (s *Store) List(f Filter) ([]Call, Stats, error) {
	if s == nil {
		return nil, Stats{}, errors.New("mcp call store is not configured")
	}
	where, args := filterSQL(f)
	listSQL := `SELECT time, request_id, actor_id, tool, device_id, workspace, success, duration_ms, error_kind
		FROM mcp_calls` + where + ` ORDER BY id DESC LIMIT ? OFFSET ?`
	limit, offset := f.Limit, f.Offset
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.Query(listSQL, append(args, limit, offset)...)
	if err != nil {
		return nil, Stats{}, err
	}
	defer rows.Close()
	var out []Call
	for rows.Next() {
		var c Call
		var success int
		var timeStr string
		if err := rows.Scan(&timeStr, &c.RequestID, &c.ActorID, &c.Tool, &c.DeviceID, &c.Workspace, &success, &c.DurationMS, &c.ErrorKind); err != nil {
			return nil, Stats{}, err
		}
		if parsed, parseErr := time.Parse(time.RFC3339Nano, timeStr); parseErr == nil {
			c.Time = parsed
		}
		c.Success = success != 0
		out = append(out, c)
	}
	stats, err := s.stats(where, args)
	if err != nil {
		return nil, Stats{}, err
	}
	return out, stats, nil
}

func (s *Store) stats(where string, args []any) (Stats, error) {
	row := s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN success=0 THEN 1 ELSE 0 END),0),
		COALESCE(CAST(AVG(duration_ms) AS INTEGER),0), COALESCE(SUM(duration_ms),0) FROM mcp_calls`+where, args...)
	var st Stats
	if err := row.Scan(&st.Total, &st.Failed, &st.AvgMS, &st.TotalMS); err != nil {
		return Stats{}, err
	}
	return st, nil
}

// Prune deletes calls older than the retention window; returns rows removed.
func (s *Store) Prune(olderThan time.Time) (int64, error) {
	if s == nil {
		return 0, nil
	}
	res, err := s.db.Exec(`DELETE FROM mcp_calls WHERE time < ?`, olderThan.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// filterSQL builds " WHERE ..." with positional args. All user input is
// bound, never concatenated.
func filterSQL(f Filter) (string, []any) {
	var clauses []string
	var args []any
	if f.Tool != "" {
		clauses = append(clauses, "tool = ?")
		args = append(args, f.Tool)
	}
	if f.ActorID != "" {
		clauses = append(clauses, "actor_id = ?")
		args = append(args, f.ActorID)
	}
	if f.DeviceID != "" {
		clauses = append(clauses, "device_id = ?")
		args = append(args, f.DeviceID)
	}
	if f.Workspace != "" {
		clauses = append(clauses, "workspace = ?")
		args = append(args, f.Workspace)
	}
	if f.Success != nil {
		clauses = append(clauses, "success = ?")
		args = append(args, boolToInt(*f.Success))
	}
	if !f.Since.IsZero() {
		clauses = append(clauses, "time >= ?")
		args = append(args, f.Since.UTC().Format(time.RFC3339Nano))
	}
	if !f.Until.IsZero() {
		clauses = append(clauses, "time <= ?")
		args = append(args, f.Until.UTC().Format(time.RFC3339Nano))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		like := "%" + q + "%"
		clauses = append(clauses, "(tool LIKE ? OR device_id LIKE ? OR workspace LIKE ? OR request_id LIKE ? OR actor_id LIKE ? OR error_kind LIKE ?)")
		args = append(args, like, like, like, like, like, like)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
