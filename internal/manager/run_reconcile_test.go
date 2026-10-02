package manager

import (
	"testing"

	"github.com/edynasty/codebridge/internal/protocol"
)

func TestRunEventBrokerMissingRequestsOnlyUnsyncedSuffix(t *testing.T) {
	b := NewRunEventBroker()
	for seq := int64(1); seq <= 3; seq++ {
		if _, _, err := b.Accept("acct", "dev", testRunEvent("run_a", seq, "harness.raw")); err != nil {
			t.Fatal(err)
		}
	}

	replay, err := b.Missing("acct", "dev", []protocol.RunHead{
		{RunID: "run_a", LastSeq: 7, Status: "running"},
		{RunID: "run_b", LastSeq: 2, Status: "completed"},
		{RunID: "run_c", LastSeq: 0, Status: "completed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(replay.Runs) != 2 {
		t.Fatalf("replay cursors = %+v, want 2", replay.Runs)
	}
	if got := replay.Runs[0]; got.RunID != "run_a" || got.AfterSeq != 3 || got.ThroughSeq != 7 {
		t.Fatalf("run_a replay = %+v", got)
	}
	if got := replay.Runs[1]; got.RunID != "run_b" || got.AfterSeq != 0 || got.ThroughSeq != 2 {
		t.Fatalf("run_b replay = %+v", got)
	}
}

func TestRunEventBrokerMissingRejectsDuplicateHeads(t *testing.T) {
	b := NewRunEventBroker()
	_, err := b.Missing("acct", "dev", []protocol.RunHead{
		{RunID: "same", LastSeq: 1},
		{RunID: "same", LastSeq: 2},
	})
	if err == nil {
		t.Fatal("Missing accepted duplicate run heads")
	}
}
