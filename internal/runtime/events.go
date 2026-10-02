package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Event page sizes. A subscriber asks for a bounded page and recovers from its
// last cursor, so the store never materializes an unbounded journal read.
const (
	defaultEventPage = 200
	maxEventPage     = 500
)

// AppendEvent writes one journal entry and returns it with its assigned
// Seq and Pos. Seq is contiguous per stream (gap detection, idempotent replay)
// and Pos is the global monotonic cursor; both are assigned inside the caller's
// transaction, so a state change and its event commit together.
func (t *Tx) AppendEvent(ev Event) (Event, error) {
	if err := requireText("event stream", ev.Stream); err != nil {
		return Event{}, err
	}
	if err := requireText("event kind", ev.Kind); err != nil {
		return Event{}, err
	}
	if ev.Source == "" {
		ev.Source = SourceRuntime
	}
	if ev.At.IsZero() {
		ev.At = time.Now()
	}
	var seq int64
	if err := t.tx.QueryRow(`SELECT COALESCE(MAX(seq), 0) + 1 FROM events WHERE stream = ?`, ev.Stream).
		Scan(&seq); err != nil {
		return Event{}, fmt.Errorf("runtime: allocate event seq: %w", err)
	}
	res, err := t.tx.Exec(`INSERT INTO events
		(stream, seq, at_ms, session_id, project_id, source, kind, turn_ref, correlation_id,
		 payload, raw_ref)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.Stream, seq, ms(ev.At), ev.SessionID, ev.ProjectID, string(ev.Source), ev.Kind,
		ev.TurnRef, ev.CorrelationID, ev.Payload, ev.RawRef)
	if err != nil {
		return Event{}, fmt.Errorf("runtime: append event: %w", err)
	}
	pos, err := res.LastInsertId()
	if err != nil {
		return Event{}, fmt.Errorf("runtime: read event position: %w", err)
	}
	ev.Seq, ev.Pos = seq, pos
	// Run.last_seq mirrors the stream head so a UI can render a run without
	// querying the journal.
	if runID, ok := runIDFromStream(ev.Stream); ok {
		if _, err := t.tx.Exec(`UPDATE runs SET last_seq = ? WHERE id = ?`, seq, runID); err != nil {
			return Event{}, fmt.Errorf("runtime: update run stream head: %w", err)
		}
	}
	return ev, nil
}

const eventColumns = `pos, stream, seq, at_ms, session_id, project_id, source, kind, turn_ref,
	correlation_id, payload, raw_ref`

func scanEvents(rows *sql.Rows) ([]Event, error) {
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var (
			ev   Event
			atMS int64
		)
		if err := rows.Scan(&ev.Pos, &ev.Stream, &ev.Seq, &atMS, &ev.SessionID, &ev.ProjectID,
			&ev.Source, &ev.Kind, &ev.TurnRef, &ev.CorrelationID, &ev.Payload, &ev.RawRef); err != nil {
			return nil, err
		}
		ev.At = timeFromMS(atMS)
		out = append(out, ev)
	}
	return out, rows.Err()
}

// EventsAfter returns journal events strictly after pos, ordered by pos, for
// callers that follow the whole journal with one cursor.
func (s *Store) EventsAfter(pos int64, limit int) ([]Event, error) {
	rows, err := s.db.Query(`SELECT `+eventColumns+` FROM events WHERE pos > ? ORDER BY pos LIMIT ?`,
		pos, eventPageLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("runtime: read events: %w", err)
	}
	events, err := scanEvents(rows)
	if err != nil {
		return nil, fmt.Errorf("runtime: read events: %w", err)
	}
	return events, nil
}

// EventsForStream returns events of one stream strictly after seq, ordered by
// seq, for replaying a single run or computer session.
func (s *Store) EventsForStream(stream string, afterSeq int64, limit int) ([]Event, error) {
	rows, err := s.db.Query(`SELECT `+eventColumns+` FROM events
		WHERE stream = ? AND seq > ? ORDER BY seq LIMIT ?`, stream, afterSeq, eventPageLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("runtime: read stream events: %w", err)
	}
	events, err := scanEvents(rows)
	if err != nil {
		return nil, fmt.Errorf("runtime: read stream events: %w", err)
	}
	return events, nil
}

// JournalHead returns the current global journal position (0 when empty).
func (s *Store) JournalHead() (int64, error) {
	return s.head(`SELECT COALESCE(MAX(pos), 0) FROM events`)
}

// StreamHead returns the highest seq of one stream (0 when empty).
func (s *Store) StreamHead(stream string) (int64, error) {
	return s.head(`SELECT COALESCE(MAX(seq), 0) FROM events WHERE stream = ?`, stream)
}

func (s *Store) head(query string, args ...any) (int64, error) {
	var head int64
	if err := s.db.QueryRow(query, args...).Scan(&head); err != nil {
		return 0, fmt.Errorf("runtime: read journal head: %w", err)
	}
	return head, nil
}

func eventPageLimit(limit int) int {
	if limit <= 0 {
		return defaultEventPage
	}
	if limit > maxEventPage {
		return maxEventPage
	}
	return limit
}

// StoreStats is a cheap snapshot of the daemon's durable state for a health
// tool. ComputerSessionsOpen counts sessions that are not closed or failed.
type StoreStats struct {
	RunsActive           int
	RunsTotal            int
	SessionsOpen         int
	ComputerSessionsOpen int
	PendingApprovals     int
	JournalPos           int64
}

// Stats returns StoreStats in one query.
func (s *Store) Stats() (StoreStats, error) {
	return s.StatsContext(context.Background())
}

// StatsContext is Stats with a caller-provided context.
func (s *Store) StatsContext(ctx context.Context) (StoreStats, error) {
	var stats StoreStats
	err := s.db.QueryRowContext(ctx, `SELECT
			(SELECT COUNT(*) FROM runs WHERE status IN ('queued', 'running')),
			(SELECT COUNT(*) FROM runs),
			(SELECT COUNT(*) FROM sessions WHERE status = 'open'),
			(SELECT COUNT(*) FROM computer_sessions WHERE state NOT IN ('closed', 'failed')),
			(SELECT COUNT(*) FROM approvals WHERE status = 'pending'),
			COALESCE((SELECT MAX(pos) FROM events), 0)`).
		Scan(&stats.RunsActive, &stats.RunsTotal, &stats.SessionsOpen,
			&stats.ComputerSessionsOpen, &stats.PendingApprovals, &stats.JournalPos)
	if err != nil {
		return StoreStats{}, fmt.Errorf("runtime: read store stats: %w", err)
	}
	return stats, nil
}

// Ping reports whether the store answers on a pooled connection.
func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errNoStore
	}
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("runtime: ping store: %w", err)
	}
	return nil
}

var errNoStore = errors.New("runtime: store is not open")
