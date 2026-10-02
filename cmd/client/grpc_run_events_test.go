package main

import (
	"testing"
	"time"

	"github.com/edynasty/codebridge/internal/agentops"
)

// A long run publishes lifecycle/raw events to the Manager independently of
// agent_status polling or an outstanding request. This is the RT-003 contract.
func TestGRPCRunEventsReachManagerWithoutPolling(t *testing.T) {
	exec := newChainExecutor("tick")
	h := newChainHarness(t, exec.exec)
	disconnect, done, issued := h.connect(h.registerEnrollment())
	awaitCredential(t, issued)

	start := decode[agentStartReply](t, h.call(t, "agent_start", map[string]any{"task": "push events"}))
	awaitStatus(t, h.runs, start.RunID, agentops.RunRunning)

	awaitCondition(t, 5*time.Second, "the Manager to receive unsolicited run events", func() bool {
		return h.runEvents.Head("default", chainDeviceID, start.RunID) >= 3
	})

	// Finish locally and observe completion only through the Manager broker;
	// there is intentionally no agent_status/agent_result polling here.
	exec.finish()
	awaitStatus(t, h.runs, start.RunID, agentops.RunCompleted)
	snap, ok := h.runs.Get(start.RunID)
	if !ok {
		t.Fatalf("run %s missing locally", start.RunID)
	}

	awaitCondition(t, 5*time.Second, "the terminal run event to reach the Manager", func() bool {
		return h.runEvents.Head("default", chainDeviceID, start.RunID) == snap.LastSeq
	})
	last, ok := h.runEvents.Last("default", chainDeviceID, start.RunID)
	if !ok {
		t.Fatal("Manager has no last event for the run")
	}
	if last.Kind != "run.completed" || last.Seq != snap.LastSeq {
		t.Fatalf("Manager last event = %+v, want run.completed seq %d", last, snap.LastSeq)
	}

	endSession(t, disconnect, done)
}
