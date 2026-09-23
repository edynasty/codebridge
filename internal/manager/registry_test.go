package manager

import (
	"context"
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

func TestRegistryListAccountScopesDevices(t *testing.T) {
	registry := NewRegistry(2)
	registry.Put(Device{ID: "dev-a", AccountID: "account-a"}, nil)
	registry.Put(Device{ID: "dev-b", AccountID: "account-b"}, nil)

	scoped := registry.ListAccount("account-a")
	if len(scoped) != 1 || scoped[0].ID != "dev-a" {
		t.Fatalf("ListAccount leaked or missed devices: %#v", scoped)
	}
	if all := registry.List(); len(all) != 2 {
		t.Fatalf("admin List should see all devices: %#v", all)
	}
}

func TestRegistryWorkspacesAccountScope(t *testing.T) {
	registry := NewRegistry(2)
	registry.Put(Device{ID: "dev-a", AccountID: "account-a", Workspaces: []protocol.Workspace{{Name: "demo"}}}, nil)

	if _, err := registry.Workspaces("account-b", "dev-a"); err == nil {
		t.Fatal("cross-account Workspaces call succeeded")
	}
	ws, err := registry.Workspaces("account-a", "dev-a")
	if err != nil {
		t.Fatalf("same-account Workspaces call failed: %v", err)
	}
	if len(ws) != 1 || ws[0].Name != "demo" {
		t.Fatalf("unexpected workspaces: %#v", ws)
	}
}

func TestRegistryCallRejectsWrongAccountLikeUnknownDevice(t *testing.T) {
	registry := NewRegistry(2)
	registry.Put(Device{ID: "dev-a", AccountID: "account-a"}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	wrongAccount, err := registry.Call(ctx, "account-b", "dev-a", protocol.AgentRequest{})
	if wrongAccount != nil || err == nil {
		t.Fatalf("cross-account Call succeeded: %v", err)
	}
	unknown, err := registry.Call(ctx, "account-b", "missing-device", protocol.AgentRequest{})
	if unknown != nil || err == nil {
		t.Fatalf("unknown-device Call succeeded: %v", err)
	}
	if err.Error() != unknownError("missing-device") {
		t.Fatalf("cross-account and unknown-device errors differ: %q vs %q", err.Error(), unknownError("missing-device"))
	}
}

func unknownError(deviceID string) string {
	return "device \"" + deviceID + "\" is offline or unknown"
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
