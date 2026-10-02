package agentops

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// RunEvent is one durable ordered event in a managed run. Seq is monotonic
// within one run and is the replay cursor used after reconnects or UI refreshes.
type RunEvent struct {
	RunID   string    `json:"run_id"`
	Seq     int64     `json:"seq"`
	At      time.Time `json:"at"`
	Source  string    `json:"source,omitempty"`
	Kind    string    `json:"kind"`
	Payload string    `json:"payload,omitempty"`
}

const (
	defaultRunSubscriberBuffer = 64
	maxRunSubscriberBuffer     = 1024
)

type runSubscriber struct {
	runID string
	ch    chan RunEvent
}

// SubscribeEvents subscribes to newly appended Runtime events. An empty
// runID receives every run. Delivery is intentionally best-effort: when a
// bounded subscriber channel is full, the event is dropped for that live
// subscriber and can be recovered later with EventsAfter.
func (m *RunManager) SubscribeEvents(runID string, buffer int) (<-chan RunEvent, func()) {
	if buffer <= 0 {
		buffer = defaultRunSubscriberBuffer
	}
	if buffer > maxRunSubscriberBuffer {
		buffer = maxRunSubscriberBuffer
	}

	ch := make(chan RunEvent, buffer)
	m.mu.Lock()
	if m.closed {
		close(ch)
		m.mu.Unlock()
		return ch, func() {}
	}
	m.subNext++
	id := m.subNext
	m.subscribers[id] = runSubscriber{runID: runID, ch: ch}
	m.mu.Unlock()

	cancel := func() {
		m.mu.Lock()
		if sub, ok := m.subscribers[id]; ok {
			delete(m.subscribers, id)
			close(sub.ch)
		}
		m.mu.Unlock()
	}
	return ch, cancel
}

// publishRuntimeEventLocked fan-outs one event to live subscribers without
// blocking the run. The RunManager write lock must already be held.
func (m *RunManager) publishRuntimeEventLocked(ev RunEvent) {
	for _, sub := range m.subscribers {
		if sub.runID != "" && sub.runID != ev.RunID {
			continue
		}
		select {
		case sub.ch <- ev:
		default:
			// Live delivery is lossy by design; EventsAfter is the replay path.
		}
	}
}

func initRunEventStore(db *sql.DB) error {
	if db == nil {
		return nil
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS agent_run_events (
		run_id   TEXT NOT NULL,
		seq      INTEGER NOT NULL,
		at_ms    INTEGER NOT NULL,
		source   TEXT NOT NULL DEFAULT '',
		kind     TEXT NOT NULL DEFAULT '',
		payload  TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (run_id, seq)
	);
	CREATE INDEX IF NOT EXISTS idx_agent_run_events_run_seq
		ON agent_run_events(run_id, seq);`); err != nil {
		return fmt.Errorf("init agent run event store: %w", err)
	}
	return nil
}

// recordInterruptedRunEvents records a durable terminal event for rows left
// queued/running by a previous client process before openRunStore marks those
// rows interrupted. It runs exactly once for a stale row because the status is
// changed immediately afterwards.
func recordInterruptedRunEvents(db *sql.DB) error {
	if db == nil {
		return nil
	}
	rows, err := db.Query(`SELECT id FROM agent_runs WHERE status IN (?, ?)`,
		string(RunQueued), string(RunRunning))
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, id := range ids {
		var seq int64
		if err := db.QueryRow(`SELECT COALESCE(MAX(seq), 0) FROM agent_run_events WHERE run_id = ?`, id).Scan(&seq); err != nil {
			return err
		}
		if _, err := db.Exec(`INSERT OR IGNORE INTO agent_run_events
			(run_id, seq, at_ms, source, kind, payload)
			VALUES (?, ?, ?, ?, ?, ?)`,
			id, seq+1, unixMilli(now), "codebridge", "run.interrupted", ""); err != nil {
			return err
		}
	}
	return nil
}

func (s *runStore) appendEvent(ev RunEvent) error {
	if s == nil {
		return nil
	}
	_, err := s.db.Exec(`INSERT OR REPLACE INTO agent_run_events
		(run_id, seq, at_ms, source, kind, payload)
		VALUES (?, ?, ?, ?, ?, ?)`,
		ev.RunID, ev.Seq, unixMilli(ev.At), ev.Source, ev.Kind, ev.Payload)
	return err
}

func (s *runStore) lastSeq(runID string) (int64, error) {
	if s == nil {
		return 0, nil
	}
	var seq int64
	if err := s.db.QueryRow(
		`SELECT COALESCE(MAX(seq), 0) FROM agent_run_events WHERE run_id = ?`,
		runID,
	).Scan(&seq); err != nil {
		return 0, err
	}
	return seq, nil
}

func (s *runStore) eventsAfter(runID string, afterSeq int64, limit int) ([]RunEvent, error) {
	if s == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = defaultRunEventCapacity
	}
	rows, err := s.db.Query(`SELECT run_id, seq, at_ms, source, kind, payload
		FROM agent_run_events
		WHERE run_id = ? AND seq > ?
		ORDER BY seq ASC
		LIMIT ?`, runID, afterSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RunEvent, 0)
	for rows.Next() {
		var (
			ev   RunEvent
			atMS int64
		)
		if err := rows.Scan(&ev.RunID, &ev.Seq, &atMS, &ev.Source, &ev.Kind, &ev.Payload); err != nil {
			return nil, err
		}
		ev.At = timeFromMilli(atMS)
		out = append(out, ev)
	}
	return out, rows.Err()
}

// EventsAfter returns ordered Runtime events strictly after afterSeq.
// Persistent stores are authoritative; in-memory mode falls back to the
// retained structured-event ring.
func (m *RunManager) EventsAfter(runID string, afterSeq int64, limit int) []RunEvent {
	if limit <= 0 {
		limit = m.eventCapacity
	}
	if m.store != nil {
		events, err := m.store.eventsAfter(runID, afterSeq, limit)
		if err == nil {
			return events
		}
		log.Printf("agent runs: load events for %s after %d: %v", runID, afterSeq, err)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	rs, ok := m.runs[runID]
	if !ok || rs.journalKept == 0 {
		return nil
	}
	start := rs.journalNext - rs.journalKept
	out := make([]RunEvent, 0, rs.journalKept)
	for i := start; i < rs.journalNext; i++ {
		ev := rs.journal[i%len(rs.journal)]
		if ev.Seq <= afterSeq {
			continue
		}
		out = append(out, ev)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// appendRuntimeEventLocked appends one event to the local cache and durable
// journal. The RunManager write lock must already be held by the caller.
func (m *RunManager) appendRuntimeEventLocked(rs *runState, source, kind, payload string) {
	if rs == nil {
		return
	}
	rs.eventSeq++
	if source == "" {
		source = "codebridge"
	}
	ev := RunEvent{
		RunID:   rs.id,
		Seq:     rs.eventSeq,
		At:      time.Now().UTC(),
		Source:  source,
		Kind:    kind,
		Payload: payload,
	}
	if len(rs.journal) > 0 {
		rs.journal[rs.journalNext%len(rs.journal)] = ev
		rs.journalNext++
		if rs.journalKept < len(rs.journal) {
			rs.journalKept++
		}
	}
	if err := m.store.appendEvent(ev); err != nil {
		log.Printf("agent runs: persist event %s/%d: %v", rs.id, ev.Seq, err)
	}
	m.publishRuntimeEventLocked(ev)
}

func runStatusEventKind(status RunStatus) string {
	switch status {
	case RunQueued:
		return "run.queued"
	case RunRunning:
		return "run.started"
	case RunCompleted:
		return "run.completed"
	case RunFailed:
		return "run.failed"
	case RunCancelled:
		return "run.cancelled"
	case RunTimeout:
		return "run.timeout"
	case RunInterrupted:
		return "run.interrupted"
	default:
		return "run.status"
	}
}
