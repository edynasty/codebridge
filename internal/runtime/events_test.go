package runtime

import (
	"errors"
	"path/filepath"
	"testing"
)

func appendEvents(t *testing.T, s *Store, stream string, count int) []Event {
	t.Helper()
	var appended []Event
	err := s.Update(func(tx *Tx) error {
		for range count {
			ev, err := tx.AppendEvent(Event{Stream: stream, Kind: "agent.message"})
			if err != nil {
				return err
			}
			appended = append(appended, ev)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("append %d events on %s: %v", count, stream, err)
	}
	return appended
}

func TestAppendEventSeqAndPos(t *testing.T) {
	s := openTemp(t)

	first := appendEvents(t, s, StreamRun("run_a"), 3)
	for i, ev := range first {
		if ev.Seq != int64(i+1) {
			t.Fatalf("run_a event %d: want seq %d, got %d", i, i+1, ev.Seq)
		}
		if ev.Pos <= 0 {
			t.Fatalf("run_a event %d: pos not assigned (%d)", i, ev.Pos)
		}
	}
	second := appendEvents(t, s, StreamRun("run_b"), 2)
	if second[0].Seq != 1 || second[1].Seq != 2 {
		t.Fatalf("run_b seqs: want 1,2 got %d,%d", second[0].Seq, second[1].Seq)
	}
	// Global pos is one monotonic sequence across streams.
	if second[0].Pos <= first[len(first)-1].Pos {
		t.Fatalf("global pos did not advance across streams: %d then %d", first[len(first)-1].Pos, second[0].Pos)
	}
	if err := ValidateContiguous(first, 0); err != nil {
		t.Fatalf("ValidateContiguous(run_a): %v", err)
	}
}

func TestEventContinuityAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	s := openAt(t, path)
	appendEvents(t, s, StreamRun("run_a"), 3)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened := openAt(t, path)
	next := appendEvents(t, reopened, StreamRun("run_a"), 2)
	if next[0].Seq != 4 || next[1].Seq != 5 {
		t.Fatalf("seqs after reopen: want 4,5 got %d,%d", next[0].Seq, next[1].Seq)
	}
	head, err := reopened.StreamHead(StreamRun("run_a"))
	if err != nil {
		t.Fatalf("StreamHead: %v", err)
	}
	if head != 5 {
		t.Fatalf("stream head: want 5, got %d", head)
	}
	journalHead, err := reopened.JournalHead()
	if err != nil {
		t.Fatalf("JournalHead: %v", err)
	}
	if journalHead != 5 {
		t.Fatalf("journal head: want 5, got %d", journalHead)
	}

	replay, err := reopened.EventsForStream(StreamRun("run_a"), 0, 0)
	if err != nil {
		t.Fatalf("EventsForStream: %v", err)
	}
	if len(replay) != 5 {
		t.Fatalf("replay length: want 5, got %d", len(replay))
	}
	if err := ValidateContiguous(replay, 0); err != nil {
		t.Fatalf("ValidateContiguous(replay): %v", err)
	}
	if err := ValidateContiguous(replay[3:], 3); err != nil {
		t.Fatalf("ValidateContiguous(from cursor 3): %v", err)
	}
	after, err := reopened.EventsAfter(3, 0)
	if err != nil {
		t.Fatalf("EventsAfter: %v", err)
	}
	if len(after) != 2 || after[0].Pos != 4 || after[1].Pos != 5 {
		t.Fatalf("EventsAfter(3): got %d events %+v", len(after), after)
	}
}

func TestFailedTransactionLeavesNoSeqGap(t *testing.T) {
	s := openTemp(t)
	appendEvents(t, s, StreamRun("run_a"), 2)

	boom := errors.New("boom")
	err := s.Update(func(tx *Tx) error {
		if _, err := tx.AppendEvent(Event{Stream: StreamRun("run_a"), Kind: "agent.message"}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Update error: want %v, got %v", boom, err)
	}

	next := appendEvents(t, s, StreamRun("run_a"), 1)
	if next[0].Seq != 3 {
		t.Fatalf("seq after a rolled back append: want 3, got %d", next[0].Seq)
	}
	head, err := s.JournalHead()
	if err != nil {
		t.Fatalf("JournalHead: %v", err)
	}
	if head != 3 {
		t.Fatalf("journal head after rollback: want 3, got %d", head)
	}
}

func TestRunLastSeqFollowsJournal(t *testing.T) {
	s := openTemp(t)
	seedGraph(t, s)
	if err := putRunOf(t, s, "run_a", RunRunning, "psn_t"); err != nil {
		t.Fatalf("PutRun: %v", err)
	}
	appendEvents(t, s, StreamRun("run_a"), 3)

	run, err := s.GetRun("run_a")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.LastSeq != 3 {
		t.Fatalf("run last_seq: want 3, got %d", run.LastSeq)
	}
	// An ordinary update must not roll the stream head back.
	run.Model = "changed"
	if err := write(s, func(tx *Tx) error { return tx.PutRun(*run) }); err != nil {
		t.Fatalf("PutRun after events: %v", err)
	}
	reread, err := s.GetRun("run_a")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if reread.LastSeq != 3 {
		t.Fatalf("run last_seq after update: want 3, got %d", reread.LastSeq)
	}
}

func TestValidateContiguousReportsGap(t *testing.T) {
	if err := ValidateContiguous(nil, 7); err != nil {
		t.Fatalf("empty slice: %v", err)
	}
	if err := ValidateContiguous([]Event{{Seq: 1}, {Seq: 2}, {Seq: 3}}, 0); err != nil {
		t.Fatalf("contiguous slice: %v", err)
	}
	if err := ValidateContiguous([]Event{{Seq: 1}, {Seq: 3}}, 0); err == nil {
		t.Fatal("gap was not reported")
	}
	if err := ValidateContiguous([]Event{{Seq: 1}, {Seq: 2}}, 2); err == nil {
		t.Fatal("replayed prefix was not reported as a gap")
	}
}

func TestEventPageLimits(t *testing.T) {
	s := openTemp(t)
	appendEvents(t, s, StreamSystem, 600)

	page, err := s.EventsAfter(0, 0)
	if err != nil {
		t.Fatalf("EventsAfter default: %v", err)
	}
	if len(page) != defaultEventPage {
		t.Fatalf("default page: want %d, got %d", defaultEventPage, len(page))
	}
	page, err = s.EventsAfter(0, 10_000)
	if err != nil {
		t.Fatalf("EventsAfter over cap: %v", err)
	}
	if len(page) != maxEventPage {
		t.Fatalf("capped page: want %d, got %d", maxEventPage, len(page))
	}
	page, err = s.EventsAfter(0, 50)
	if err != nil {
		t.Fatalf("EventsAfter explicit: %v", err)
	}
	if len(page) != 50 {
		t.Fatalf("explicit page: want 50, got %d", len(page))
	}
}

func TestStats(t *testing.T) {
	s := openTemp(t)
	seedGraph(t, s)
	if err := putRunOf(t, s, "run_a", RunRunning, "psn_t"); err != nil {
		t.Fatalf("PutRun: %v", err)
	}
	mustWrite(t, s, func(tx *Tx) error {
		if err := tx.PutComputerSession(ComputerSession{
			ID: "cmp_open", SessionID: "ses_t", Target: "display:1", State: ComputerActive,
			Controller: Controller{Kind: ControllerNone}, CreatedAt: testNow, UpdatedAt: testNow,
		}); err != nil {
			return err
		}
		if err := tx.PutComputerSession(ComputerSession{
			ID: "cmp_closed", SessionID: "ses_t", Target: "display:1", State: ComputerClosed,
			Controller: Controller{Kind: ControllerNone}, CreatedAt: testNow, UpdatedAt: testNow,
		}); err != nil {
			return err
		}
		if err := tx.PutApproval(Approval{ID: "approval-1", Verb: VerbComputerInput, Status: ApprovalPending,
			Channel: ApprovalLocalUI, RequestedAt: testNow}); err != nil {
			return err
		}
		return tx.PutApproval(Approval{ID: "approval-2", Verb: VerbComputerInput, Status: ApprovalDenied,
			Channel: ApprovalLocalUI, RequestedAt: testNow})
	})

	stats, err := s.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.RunsActive != 1 || stats.RunsTotal != 1 || stats.SessionsOpen != 1 ||
		stats.ComputerSessionsOpen != 1 || stats.PendingApprovals != 1 || stats.JournalPos != 0 {
		t.Fatalf("stats: %+v", stats)
	}

	appendEvents(t, s, StreamRun("run_a"), 2)
	stats, err = s.Stats()
	if err != nil {
		t.Fatalf("Stats after events: %v", err)
	}
	if stats.JournalPos != 2 {
		t.Fatalf("journal pos: want 2, got %d", stats.JournalPos)
	}
}

func TestPingAndMissingIDs(t *testing.T) {
	s := openTemp(t)
	if err := s.Ping(t.Context()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	for name, err := range map[string]error{
		"project":          errorsOrNil(s.GetProject("prj_missing")),
		"session":          errorsOrNil(s.GetSession("ses_missing")),
		"run":              errorsOrNil(s.GetRun("run_missing")),
		"provider session": errorsOrNil(s.GetProviderSession("psn_missing")),
		"computer session": errorsOrNil(s.GetComputerSession("cmp_missing")),
		"artifact":         errorsOrNil(s.GetArtifact("art_missing")),
		"policy rule":      errorsOrNil(s.GetPolicyRule("rule-missing")),
		"grant":            errorsOrNil(s.GetGrant("grant-missing")),
		"approval":         errorsOrNil(s.GetApproval("approval-missing")),
	} {
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s: want ErrNotFound, got %v", name, err)
		}
	}
}

// errorsOrNil keeps the table above readable by turning a value into its error.
func errorsOrNil[T any](_ T, err error) error { return err }

func TestComputerControllerEventsAreJournaled(t *testing.T) {
	s := openTemp(t)
	seedGraph(t, s)
	mustWrite(t, s, func(tx *Tx) error {
		return tx.PutComputerSession(ComputerSession{
			ID: "cmp_t", SessionID: "ses_t", Target: "display:1", State: ComputerActive,
			Controller: Controller{Kind: ControllerNone}, CreatedAt: testNow, UpdatedAt: testNow,
		})
	})
	if _, _, err := s.SetComputerController(ControllerChange{
		ComputerSessionID: "cmp_t", Controller: Controller{Kind: ControllerAgent, Holder: "run_1"}, At: testNow,
	}); err != nil {
		t.Fatalf("SetComputerController: %v", err)
	}
	events, err := s.EventsForStream(StreamComputer("cmp_t"), 0, 0)
	if err != nil {
		t.Fatalf("EventsForStream: %v", err)
	}
	if len(events) != 1 || events[0].Kind != "computer.controller.changed" {
		t.Fatalf("controller events: %+v", events)
	}
	if events[0].SessionID != "ses_t" || events[0].At.IsZero() {
		t.Fatalf("controller event fields: %+v", events[0])
	}
}
