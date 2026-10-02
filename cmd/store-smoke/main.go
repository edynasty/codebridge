// Command store-smoke exercises the CodeBridge V2 runtime store end to end:
// schema creation, one-transaction state change plus journal event, the
// active-provider-run constraint, run state-machine rejection, per-stream seq /
// global pos continuity across a reopen, replay pages and store stats.
//
// It is the Phase 0 store smoke referenced by
// docs/v2/evidence/phase0-store.md:
//
//	go run ./cmd/store-smoke                 # fresh temp dir, removed on exit
//	go run ./cmd/store-smoke -keep           # keep the store for inspection
//	go run ./cmd/store-smoke -path /tmp/cb/runtime.db
//
// Every step prints an "ok" line and the command exits non-zero on the first
// failure, so a parent integration run can use it as a gate. It only touches
// the path it is given and never the real Application Support store.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/edynasty/codebridge/internal/runtime"
)

func main() {
	path := flag.String("path", "", "runtime database path (default: fresh temporary directory)")
	keep := flag.Bool("keep", false, "keep a temporary store directory after the run")
	flag.Parse()
	if err := run(*path, *keep); err != nil {
		fmt.Fprintf(os.Stderr, "store-smoke: FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("store-smoke: PASS")
}

func run(path string, keep bool) error {
	if path == "" {
		dir, err := os.MkdirTemp("", "codebridge-store-smoke-")
		if err != nil {
			return err
		}
		if !keep {
			defer func() { _ = os.RemoveAll(dir) }()
		}
		path = filepath.Join(dir, "runtime.db")
	}

	defaultDir, err := runtime.DefaultDir()
	if err != nil {
		return err
	}
	defaultPath, err := runtime.DefaultPath()
	if err != nil {
		return err
	}
	step("schema v%d; default store dir %s; default db %s", runtime.SchemaVersion, defaultDir, defaultPath)

	s, err := runtime.Open(path)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	pingErr := s.Ping(ctx)
	cancel()
	if pingErr != nil {
		return pingErr
	}
	step("open + ping %s", path)

	now := time.Now()
	if err := seed(s, now); err != nil {
		return err
	}
	step("seed project/session/provider session/run + run.running event in one transaction")

	runHead, err := runConstraints(s, now)
	if err != nil {
		return err
	}
	step("rejected: second active run on one provider session, and completed -> running; run_a is terminal")

	streamHead, err := journal(s, runHead)
	if err != nil {
		return err
	}
	step("journal: run_smoke_b has %d contiguous events, run.last_seq follows the stream head", streamHead)

	if err := computerSession(s, now); err != nil {
		return err
	}
	step("computer session: controller change journaled with controller_epoch 1")

	if err := stats(s); err != nil {
		return err
	}

	if err := s.Close(); err != nil {
		return err
	}
	step("closed store")

	reopened, err := runtime.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = reopened.Close() }()
	if err := resumeAfterReopen(reopened, streamHead, now); err != nil {
		return err
	}
	step("reopened store: per-stream seq and global pos continue where they stopped")
	return nil
}

func seed(s *runtime.Store, now time.Time) error {
	return s.Update(func(tx *runtime.Tx) error {
		if err := tx.PutProject(runtime.Project{
			ID:        "prj_smoke",
			Name:      "store smoke",
			Roots:     []runtime.ProjectRoot{{Path: "/tmp/store-smoke", Writable: true}},
			CreatedAt: now,
			UpdatedAt: now,
		}); err != nil {
			return err
		}
		if err := tx.PutSession(runtime.Session{
			ID:          "ses_smoke",
			ProjectID:   "prj_smoke",
			Title:       "store smoke",
			CallerClass: runtime.CallerLocalUI,
			Status:      runtime.SessionOpen,
			CreatedAt:   now,
			UpdatedAt:   now,
		}); err != nil {
			return err
		}
		if err := tx.PutProviderSession(runtime.ProviderSession{
			ID:         "psn_smoke",
			Provider:   "omp",
			NativeID:   "smoke-native",
			Locator:    "/tmp/store-smoke/omp.jsonl",
			ProjectID:  "prj_smoke",
			CanResume:  true,
			CanHistory: true,
			CreatedAt:  now,
			UpdatedAt:  now,
		}); err != nil {
			return err
		}
		if err := tx.PutRun(runtime.Run{
			ID:                "run_smoke_a",
			SessionID:         "ses_smoke",
			ProjectID:         "prj_smoke",
			Kind:              runtime.RunAgent,
			Provider:          "omp",
			Model:             "smoke-model",
			Status:            runtime.RunRunning,
			ProviderSessionID: "psn_smoke",
			ProcessPID:        os.Getpid(),
			ProcessStartedAt:  now,
			CreatedAt:         now,
			StartedAt:         now,
		}); err != nil {
			return err
		}
		_, err := tx.AppendEvent(runtime.Event{
			Stream:    runtime.StreamRun("run_smoke_a"),
			At:        now,
			SessionID: "ses_smoke",
			ProjectID: "prj_smoke",
			Source:    runtime.SourceRuntime,
			Kind:      "run.running",
		})
		return err
	})
}

// runConstraints proves the store-side invariants: one active run per provider
// session, and the V1 run state machine (a terminal run never falls back).
func runConstraints(s *runtime.Store, now time.Time) (int64, error) {
	conflict := s.Update(func(tx *runtime.Tx) error {
		return tx.PutRun(runtime.Run{
			ID:                "run_smoke_conflict",
			SessionID:         "ses_smoke",
			Kind:              runtime.RunAgent,
			Status:            runtime.RunRunning,
			ProviderSessionID: "psn_smoke",
			CreatedAt:         now,
			StartedAt:         now,
		})
	})
	if conflict == nil {
		return 0, errors.New("active-provider-run constraint: a second running run was accepted")
	}

	if _, _, err := s.TransitionRun(runtime.RunTransition{
		RunID:  "run_smoke_a",
		Status: runtime.RunCompleted,
		At:     now,
	}); err != nil {
		return 0, err
	}
	if _, _, err := s.TransitionRun(runtime.RunTransition{
		RunID:  "run_smoke_a",
		Status: runtime.RunRunning,
		At:     now,
	}); !errors.Is(err, runtime.ErrIllegalTransition) {
		return 0, fmt.Errorf("completed -> running: want ErrIllegalTransition, got %v", err)
	}

	// The provider session is free again now that run_smoke_a is terminal; the
	// canonical start is a queued row with its event, then a transition.
	if err := s.Update(func(tx *runtime.Tx) error {
		if err := tx.PutRun(runtime.Run{
			ID:                "run_smoke_b",
			SessionID:         "ses_smoke",
			ProjectID:         "prj_smoke",
			Kind:              runtime.RunAgent,
			Status:            runtime.RunQueued,
			ProviderSessionID: "psn_smoke",
			CreatedAt:         now,
		}); err != nil {
			return err
		}
		_, err := tx.AppendEvent(runtime.Event{
			Stream:    runtime.StreamRun("run_smoke_b"),
			At:        now,
			SessionID: "ses_smoke",
			ProjectID: "prj_smoke",
			Kind:      "run.queued",
		})
		return err
	}); err != nil {
		return 0, err
	}
	started, _, err := s.TransitionRun(runtime.RunTransition{
		RunID:  "run_smoke_b",
		Status: runtime.RunRunning,
		At:     now,
	})
	if err != nil {
		return 0, err
	}
	return started.LastSeq, nil
}

func journal(s *runtime.Store, runHead int64) (int64, error) {
	stream := runtime.StreamRun("run_smoke_b")
	start := runHead
	err := s.Update(func(tx *runtime.Tx) error {
		for i := 1; i <= 3; i++ {
			if _, err := tx.AppendEvent(runtime.Event{
				Stream:    stream,
				At:        time.Now(),
				SessionID: "ses_smoke",
				Source:    runtime.SourceAgent,
				Kind:      "agent.message",
				Payload:   fmt.Sprintf(`{"n":%d}`, i),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	head, err := s.StreamHead(stream)
	if err != nil {
		return 0, err
	}
	if want := start + 3; head != want {
		return 0, fmt.Errorf("stream head: want %d, got %d", want, head)
	}
	events, err := s.EventsForStream(stream, 0, 0)
	if err != nil {
		return 0, err
	}
	if int64(len(events)) != head {
		return 0, fmt.Errorf("stream has %d events but head is %d", len(events), head)
	}
	if err := runtime.ValidateContiguous(events, 0); err != nil {
		return 0, err
	}
	run, err := s.GetRun("run_smoke_b")
	if err != nil {
		return 0, err
	}
	if run.LastSeq != head {
		return 0, fmt.Errorf("run last_seq: want %d, got %d", head, run.LastSeq)
	}
	return head, nil
}

func computerSession(s *runtime.Store, now time.Time) error {
	if err := s.Update(func(tx *runtime.Tx) error {
		return tx.PutComputerSession(runtime.ComputerSession{
			ID:           "cmp_smoke",
			SessionID:    "ses_smoke",
			RunID:        "run_smoke_b",
			Target:       "display:1",
			State:        runtime.ComputerActive,
			Controller:   runtime.Controller{Kind: runtime.ControllerNone},
			CanObserve:   true,
			CanInput:     true,
			GrantedVerbs: []runtime.Verb{runtime.VerbComputerObserve, runtime.VerbComputerInput},
			AppSelectors: []string{"com.apple.TextEdit"},
			CreatedAt:    now,
			UpdatedAt:    now,
		})
	}); err != nil {
		return err
	}
	session, event, err := s.SetComputerController(runtime.ControllerChange{
		ComputerSessionID: "cmp_smoke",
		Controller:        runtime.Controller{Kind: runtime.ControllerAgent, Holder: "run_smoke_b"},
		At:                now,
		Payload:           `{"to":"agent"}`,
	})
	if err != nil {
		return err
	}
	if session.ControllerEpoch != 1 {
		return fmt.Errorf("controller epoch: want 1, got %d", session.ControllerEpoch)
	}
	if event.Kind != "computer.controller.changed" || event.Seq != 1 {
		return fmt.Errorf("controller event: got kind %q seq %d", event.Kind, event.Seq)
	}
	return nil
}

func stats(s *runtime.Store) error {
	got, err := s.Stats()
	if err != nil {
		return err
	}
	want := runtime.StoreStats{
		RunsActive: 1, RunsTotal: 2, SessionsOpen: 1, ComputerSessionsOpen: 1, PendingApprovals: 0,
	}
	journalHead, err := s.JournalHead()
	if err != nil {
		return err
	}
	want.JournalPos = journalHead
	if got != want {
		return fmt.Errorf("stats: want %+v, got %+v", want, got)
	}
	step("stats runs_active=%d runs_total=%d sessions_open=%d computer_sessions_open=%d pending_approvals=%d journal_pos=%d",
		got.RunsActive, got.RunsTotal, got.SessionsOpen, got.ComputerSessionsOpen,
		got.PendingApprovals, got.JournalPos)
	return nil
}

func resumeAfterReopen(s *runtime.Store, streamHead int64, now time.Time) error {
	var appended runtime.Event
	err := s.Update(func(tx *runtime.Tx) error {
		ev, err := tx.AppendEvent(runtime.Event{
			Stream:    runtime.StreamRun("run_smoke_b"),
			At:        now,
			SessionID: "ses_smoke",
			Kind:      "agent.message",
		})
		appended = ev
		return err
	})
	if err != nil {
		return err
	}
	if appended.Seq != streamHead+1 {
		return fmt.Errorf("seq after reopen: want %d, got %d", streamHead+1, appended.Seq)
	}
	events, err := s.EventsForStream(runtime.StreamRun("run_smoke_b"), 0, 0)
	if err != nil {
		return err
	}
	if err := runtime.ValidateContiguous(events, 0); err != nil {
		return err
	}
	journalHead, err := s.JournalHead()
	if err != nil {
		return err
	}
	page, err := s.EventsAfter(journalHead-2, 0)
	if err != nil {
		return err
	}
	step("after reopen: appended seq %d pos %d, %d events on the stream, journal head %d, replay page %d",
		appended.Seq, appended.Pos, len(events), journalHead, len(page))
	return nil
}

func step(format string, args ...any) {
	fmt.Printf("ok   "+format+"\n", args...)
}
