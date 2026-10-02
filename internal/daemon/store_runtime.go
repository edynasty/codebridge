package daemon

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/edynasty/codebridge/internal/runtime"
)

// This file is the only place the daemon core reaches the concrete runtime
// store. Everything else depends on the narrow Store port in store.go, so the
// daemon and the store stay independently replaceable.
//
// The adapter maps internal/runtime's types onto the daemon-local port:
// runtime.Event -> JournalEvent and runtime.StoreStats -> StoreStats.

// runtimeStoreAdapter adapts *runtime.Store to Store.
type runtimeStoreAdapter struct {
	store *runtime.Store
}

// OpenRuntimeStore opens (creating and migrating when needed) the durable
// runtime database at path and returns it as the daemon's store port.
func OpenRuntimeStore(path string) (Store, error) {
	s, err := runtime.Open(path)
	if err != nil {
		return nil, err
	}
	return &runtimeStoreAdapter{store: s}, nil
}

// DefaultRuntimeDBPath is the durable database location for a data directory.
// It equals runtime.DefaultPath() for the default data directory.
func DefaultRuntimeDBPath(dataDir string) string {
	return filepath.Join(dataDir, "runtime.db")
}

func (a *runtimeStoreAdapter) Close() error { return a.store.Close() }

func (a *runtimeStoreAdapter) Ping(ctx context.Context) error { return a.store.Ping(ctx) }

func (a *runtimeStoreAdapter) SchemaVersion() int { return runtime.SchemaVersion }

func (a *runtimeStoreAdapter) JournalHead() (int64, error) { return a.store.JournalHead() }

func (a *runtimeStoreAdapter) EventsAfter(pos int64, limit int) ([]JournalEvent, error) {
	events, err := a.store.EventsAfter(pos, limit)
	if err != nil {
		return nil, err
	}
	return toJournalEvents(events), nil
}

func (a *runtimeStoreAdapter) EventsForStream(stream string, afterSeq int64, limit int) ([]JournalEvent, error) {
	events, err := a.store.EventsForStream(stream, afterSeq, limit)
	if err != nil {
		return nil, err
	}
	return toJournalEvents(events), nil
}

func (a *runtimeStoreAdapter) Stats() (StoreStats, error) {
	stats, err := a.store.Stats()
	if err != nil {
		return StoreStats{}, err
	}
	return StoreStats{
		SchemaVersion:        runtime.SchemaVersion,
		RunsActive:           stats.RunsActive,
		RunsTotal:            stats.RunsTotal,
		SessionsOpen:         stats.SessionsOpen,
		ComputerSessionsOpen: stats.ComputerSessionsOpen,
		PendingApprovals:     stats.PendingApprovals,
		JournalPos:           stats.JournalPos,
	}, nil
}

func toJournalEvents(events []runtime.Event) []JournalEvent {
	out := make([]JournalEvent, 0, len(events))
	for _, ev := range events {
		je := JournalEvent{
			Pos:           ev.Pos,
			Stream:        ev.Stream,
			Seq:           ev.Seq,
			Kind:          ev.Kind,
			Source:        string(ev.Source),
			At:            ev.At,
			SessionID:     ev.SessionID,
			ProjectID:     ev.ProjectID,
			CorrelationID: ev.CorrelationID,
			RawRef:        ev.RawRef,
		}
		if id, ok := strings.CutPrefix(ev.Stream, "run:"); ok {
			je.RunID = id
		}
		if ev.Payload != "" && json.Valid([]byte(ev.Payload)) {
			je.Payload = json.RawMessage(ev.Payload)
		}
		if je.At.IsZero() {
			je.At = time.Time{}
		}
		out = append(out, je)
	}
	return out
}
