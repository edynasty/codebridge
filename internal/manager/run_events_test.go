package manager

import (
	"testing"
	"time"

	"github.com/edynasty/codebridge/internal/protocol"
)

func testRunEvent(run string, seq int64, kind string) protocol.RunEvent {
	return protocol.RunEvent{
		RunID:  run,
		Seq:    seq,
		At:     time.Unix(seq, 0).UTC(),
		Source: "omp",
		Kind:   kind,
	}
}

func TestRunEventBrokerAcceptsOrderedEventsIdempotently(t *testing.T) {
	b := NewRunEventBroker()

	for seq := int64(1); seq <= 3; seq++ {
		head, accepted, err := b.Accept("acct", "dev", testRunEvent("run_a", seq, "harness.raw"))
		if err != nil {
			t.Fatalf("accept %d: %v", seq, err)
		}
		if !accepted || head != seq {
			t.Fatalf("accept %d = head %d accepted %v", seq, head, accepted)
		}
	}
	head, accepted, err := b.Accept("acct", "dev", testRunEvent("run_a", 2, "harness.raw"))
	if err != nil {
		t.Fatal(err)
	}
	if accepted || head != 3 {
		t.Fatalf("duplicate = head %d accepted %v, want 3 false", head, accepted)
	}
	if got := b.Head("acct", "dev", "run_a"); got != 3 {
		t.Fatalf("head = %d, want 3", got)
	}
}

func TestRunEventBrokerClosesOutOfOrderGap(t *testing.T) {
	b := NewRunEventBroker()

	head, accepted, err := b.Accept("acct", "dev", testRunEvent("run_a", 2, "harness.raw"))
	if err != nil || !accepted || head != 0 {
		t.Fatalf("seq2 = head %d accepted %v err %v", head, accepted, err)
	}
	head, accepted, err = b.Accept("acct", "dev", testRunEvent("run_a", 1, "run.queued"))
	if err != nil || !accepted || head != 2 {
		t.Fatalf("seq1 = head %d accepted %v err %v, want head 2", head, accepted, err)
	}
	last, ok := b.Last("acct", "dev", "run_a")
	if !ok || last.Seq != 2 {
		t.Fatalf("last = %+v ok=%v, want seq2", last, ok)
	}
}

func TestRunEventBrokerScopesByAccountDeviceAndRun(t *testing.T) {
	b := NewRunEventBroker()
	cases := [][3]string{
		{"a1", "d1", "r1"},
		{"a1", "d2", "r1"},
		{"a2", "d1", "r1"},
		{"a1", "d1", "r2"},
	}
	for _, key := range cases {
		if _, _, err := b.Accept(key[0], key[1], testRunEvent(key[2], 1, "run.queued")); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range cases {
		if got := b.Head(key[0], key[1], key[2]); got != 1 {
			t.Fatalf("head %v = %d, want 1", key, got)
		}
	}
	if got := b.Head("missing", "d1", "r1"); got != 0 {
		t.Fatalf("missing head = %d, want 0", got)
	}
}

func TestRunEventBrokerRejectsInvalidEvents(t *testing.T) {
	b := NewRunEventBroker()
	for _, tc := range []struct {
		name    string
		account string
		device  string
		ev      protocol.RunEvent
	}{
		{"account", "", "d", testRunEvent("r", 1, "x")},
		{"device", "a", "", testRunEvent("r", 1, "x")},
		{"run", "a", "d", testRunEvent("", 1, "x")},
		{"seq", "a", "d", testRunEvent("r", 0, "x")},
		{"kind", "a", "d", testRunEvent("r", 1, "")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := b.Accept(tc.account, tc.device, tc.ev); err == nil {
				t.Fatal("Accept succeeded, want validation error")
			}
		})
	}
}
