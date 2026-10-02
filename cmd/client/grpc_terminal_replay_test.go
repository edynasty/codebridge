package main

import (
	"testing"
	"time"

	"github.com/edynasty/codebridge/internal/agentops"
	"github.com/edynasty/codebridge/internal/authstore"
)

// A run may finish while the Client has no gRPC session at all. Its terminal
// event must survive locally and be replayed after the next reconnect.
func TestGRPCReconnectReplaysTerminalEventCompletedWhileOffline(t *testing.T) {
	exec := newChainExecutor("offline")
	h := newChainHarness(t, exec.exec)
	disconnect, done, issued := h.connect(h.registerEnrollment())
	credential := awaitCredential(t, issued)

	start := decode[agentStartReply](t, h.call(t, "agent_start", map[string]any{"task": "finish offline"}))
	awaitStatus(t, h.runs, start.RunID, agentops.RunRunning)
	awaitCondition(t, 5*time.Second, "initial runtime events at the Manager", func() bool {
		return h.runEvents.Head(authstore.DefaultAccount, chainDeviceID, start.RunID) >= 3
	})

	endSession(t, disconnect, done)
	waitForDeviceState(t, h.registry, chainDeviceID, false)
	headBeforeFinish := h.runEvents.Head(authstore.DefaultAccount, chainDeviceID, start.RunID)

	exec.finish()
	awaitStatus(t, h.runs, start.RunID, agentops.RunCompleted)
	local, ok := h.runs.Get(start.RunID)
	if !ok {
		t.Fatalf("run %s missing locally", start.RunID)
	}
	if local.LastSeq <= headBeforeFinish {
		t.Fatalf("local terminal seq %d did not advance beyond Manager head %d", local.LastSeq, headBeforeFinish)
	}
	if last, ok := h.runEvents.Last(authstore.DefaultAccount, chainDeviceID, start.RunID); ok && last.Kind == "run.completed" {
		t.Fatalf("Manager saw terminal event before reconnect: %+v", last)
	}

	reconnect, reconnectDone, _ := h.connect(h.registerCredential(credential))
	awaitCondition(t, 5*time.Second, "offline terminal event replay", func() bool {
		return h.runEvents.Head(authstore.DefaultAccount, chainDeviceID, start.RunID) == local.LastSeq
	})
	last, ok := h.runEvents.Last(authstore.DefaultAccount, chainDeviceID, start.RunID)
	if !ok || last.Kind != "run.completed" || last.Seq != local.LastSeq {
		t.Fatalf("replayed terminal event = %+v ok=%v, want run.completed seq %d", last, ok, local.LastSeq)
	}

	endSession(t, reconnect, reconnectDone)
}
