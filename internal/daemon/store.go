package daemon

import (
	"context"
	"encoding/json"
	"time"
)

// JournalEvent is the daemon-local projection of one runtime journal entry
// (architecture.md §8). internal/runtime's concrete Event is adapted to this
// shape by the integration owner so the daemon core never imports the store.
type JournalEvent struct {
	Pos           int64           `json:"pos"`
	Stream        string          `json:"stream"`
	Seq           int64           `json:"seq"`
	Kind          string          `json:"kind"`
	Source        string          `json:"source"`
	At            time.Time       `json:"at"`
	SessionID     string          `json:"session_id,omitempty"`
	ProjectID     string          `json:"project_id,omitempty"`
	RunID         string          `json:"run_id,omitempty"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	RawRef        string          `json:"raw_ref,omitempty"`
}

// StoreStats is the read model behind host.health, runtime.health and the MCP
// health tool.
type StoreStats struct {
	SchemaVersion        int   `json:"schema_version"`
	RunsActive           int   `json:"runs_active"`
	RunsTotal            int   `json:"runs_total"`
	SessionsOpen         int   `json:"sessions_open"`
	ComputerSessionsOpen int   `json:"computer_sessions_open"`
	PendingApprovals     int   `json:"pending_approvals"`
	JournalPos           int64 `json:"journal_pos"`
}

// Store is the narrow runtime-store port the daemon consumes. It is satisfied
// by an adapter over internal/runtime's *Store; the daemon never imports that
// package, so store and daemon stay independently replaceable.
type Store interface {
	// Close releases the store.
	Close() error
	// Ping is a cheap liveness check.
	Ping(ctx context.Context) error
	// SchemaVersion reports the durable schema version.
	SchemaVersion() int
	// JournalHead reports the highest committed global position (0 when empty).
	JournalHead() (int64, error)
	// EventsAfter returns events with pos > after, oldest first, bounded.
	EventsAfter(pos int64, limit int) ([]JournalEvent, error)
	// EventsForStream returns events on one stream with seq > afterSeq.
	EventsForStream(stream string, afterSeq int64, limit int) ([]JournalEvent, error)
	// Stats returns the runtime counters.
	Stats() (StoreStats, error)
}

// storeHealth renders the store block shared by both health surfaces.
func storeHealth(store Store) map[string]any {
	out := map[string]any{"wired": store != nil}
	if store == nil {
		return out
	}
	out["schema_version"] = store.SchemaVersion()
	if stats, err := store.Stats(); err == nil {
		out["stats"] = stats
	} else {
		out["error"] = err.Error()
	}
	return out
}
