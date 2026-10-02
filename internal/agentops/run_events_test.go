package agentops

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestRunEventJournalPersistsAndReplays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.db")
	exec := func(_ context.Context, spec RunSpec, _ string, onEvent func(string)) (SubagentResult, error) {
		onEvent("alpha")
		onEvent("beta")
		return SubagentResult{Client: spec.Client, Status: "completed", Output: "done"}, nil
	}

	m := NewRunManager(WithStore(path), WithExecutor(exec))
	run, err := m.Start(context.Background(), RunSpec{Workspace: "/work", Task: "journal", Client: "omp"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	final, err := m.Wait(ctx, run.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if final.Status != RunCompleted {
		t.Fatalf("status = %s, want %s", final.Status, RunCompleted)
	}

	events := m.EventsAfter(run.ID, 0, 100)
	wantKinds := []string{"run.queued", "run.started", "harness.raw", "harness.raw", "run.completed"}
	if len(events) != len(wantKinds) {
		t.Fatalf("events = %d, want %d: %+v", len(events), len(wantKinds), events)
	}
	for i, want := range wantKinds {
		if events[i].Seq != int64(i+1) {
			t.Fatalf("event %d seq = %d, want %d", i, events[i].Seq, i+1)
		}
		if events[i].Kind != want {
			t.Fatalf("event %d kind = %q, want %q", i, events[i].Kind, want)
		}
		if events[i].RunID != run.ID {
			t.Fatalf("event %d run = %q, want %q", i, events[i].RunID, run.ID)
		}
		if events[i].At.IsZero() {
			t.Fatalf("event %d has zero timestamp", i)
		}
	}
	if events[2].Payload != "alpha" || events[3].Payload != "beta" {
		t.Fatalf("raw payloads = %q, %q", events[2].Payload, events[3].Payload)
	}
	if final.LastSeq != int64(len(wantKinds)) {
		t.Fatalf("last seq = %d, want %d", final.LastSeq, len(wantKinds))
	}

	if err := m.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened := NewRunManager(WithStore(path))
	t.Cleanup(func() { _ = reopened.Close() })

	replayed := reopened.EventsAfter(run.ID, 0, 100)
	if len(replayed) != len(wantKinds) {
		t.Fatalf("replayed events = %d, want %d: %+v", len(replayed), len(wantKinds), replayed)
	}
	for i := range replayed {
		if replayed[i].Seq != events[i].Seq || replayed[i].Kind != events[i].Kind || replayed[i].Payload != events[i].Payload {
			t.Fatalf("replayed event %d = %+v, want %+v", i, replayed[i], events[i])
		}
	}

	tail := reopened.EventsAfter(run.ID, 2, 2)
	if len(tail) != 2 || tail[0].Seq != 3 || tail[1].Seq != 4 {
		t.Fatalf("tail = %+v, want seq 3,4", tail)
	}

	snap, ok := reopened.Get(run.ID)
	if !ok {
		t.Fatalf("run %s missing after restart", run.ID)
	}
	if snap.LastSeq != int64(len(wantKinds)) {
		t.Fatalf("reopened last seq = %d, want %d", snap.LastSeq, len(wantKinds))
	}
}

func TestRunEventStoreMigratesOldDatabaseAndRecordsInterrupted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old-runs.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE agent_runs (
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
	CREATE INDEX idx_agent_runs_started ON agent_runs(started_at DESC);
	INSERT INTO agent_runs
		(id, workspace, client, task, status, started_at, output, event_count)
		VALUES ('run_old', '/work', 'omp', 'old task', 'running', ?, 'partial', 7);`,
		time.Now().Add(-time.Minute).UnixMilli()); err != nil {
		_ = db.Close()
		t.Fatalf("seed old database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close old database: %v", err)
	}

	m := NewRunManager(WithStore(path))
	t.Cleanup(func() { _ = m.Close() })

	snap, ok := m.Get("run_old")
	if !ok {
		t.Fatal("old run missing after migration")
	}
	if snap.Status != RunInterrupted {
		t.Fatalf("old run status = %s, want %s", snap.Status, RunInterrupted)
	}
	events := m.EventsAfter("run_old", 0, 10)
	if len(events) != 1 {
		t.Fatalf("interrupted events = %d, want 1: %+v", len(events), events)
	}
	if events[0].Seq != 1 || events[0].Kind != "run.interrupted" || events[0].Source != "codebridge" {
		t.Fatalf("interrupted event = %+v", events[0])
	}
}
