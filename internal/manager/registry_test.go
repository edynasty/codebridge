package manager

import (
	"testing"
	"time"

	"github.com/edynasty/codebridge/internal/protocol"
)

func TestRegistryRemoveFailsPendingCalls(t *testing.T) {
	registry := NewRegistry(2)
	conn := registry.Put(Device{ID: "dev-1"}, nil)
	wait := addPendingForTest(conn, "req-1")

	registry.Remove("dev-1", conn)

	assertPendingFailure(t, wait, "agent disconnected")
	if _, ok := registry.Get("dev-1"); ok {
		t.Fatal("device remained registered after Remove")
	}
}

func TestRegistryReplacementFailsOldPendingCalls(t *testing.T) {
	registry := NewRegistry(2)
	old := registry.Put(Device{ID: "dev-1"}, nil)
	wait := addPendingForTest(old, "req-1")

	replacement := registry.Put(Device{ID: "dev-1"}, nil)

	assertPendingFailure(t, wait, "agent connection replaced")
	got, ok := registry.Get("dev-1")
	if !ok || got != replacement {
		t.Fatal("replacement connection was not registered")
	}
}

func TestRegistryDisconnectFailsPendingCalls(t *testing.T) {
	registry := NewRegistry(2)
	conn := registry.Put(Device{ID: "dev-1"}, nil)
	wait := addPendingForTest(conn, "req-1")

	if !registry.Disconnect("dev-1") {
		t.Fatal("Disconnect returned false for registered device")
	}

	assertPendingFailure(t, wait, "agent disconnected")
}

func addPendingForTest(conn *AgentConn, id string) <-chan protocol.AgentResponse {
	ch := make(chan protocol.AgentResponse, 1)
	conn.pendMu.Lock()
	conn.pend[id] = pendingCall{ch: ch}
	conn.pendMu.Unlock()
	return ch
}

func assertPendingFailure(t *testing.T, wait <-chan protocol.AgentResponse, want string) {
	t.Helper()
	select {
	case resp := <-wait:
		if resp.OK || resp.Error != want {
			t.Fatalf("unexpected pending response: %#v", resp)
		}
	case <-time.After(time.Second):
		t.Fatalf("pending call was not failed with %q", want)
	}
}
