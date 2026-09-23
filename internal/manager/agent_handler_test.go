package manager

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/edynasty/codebridge/internal/authstore"
	"github.com/edynasty/codebridge/internal/protocol"
	"github.com/gorilla/websocket"
)

func TestSanitizeRegistrationRemovesPhysicalPaths(t *testing.T) {
	reg := protocol.RegisterRequest{
		DeviceID:   " mac-1 ",
		DeviceName: " Mac ",
		Version:    " 0.3.0 ",
		Workspaces: []protocol.Workspace{{Name: " pms ", Path: "/Users/me/secret/pms"}},
	}
	if err := sanitizeRegistration(&reg); err != nil {
		t.Fatal(err)
	}
	if reg.DeviceID != "mac-1" || reg.DeviceName != "Mac" || reg.Version != "0.3.0" {
		t.Fatalf("registration was not normalized: %#v", reg)
	}
	if got := reg.Workspaces[0]; got.Name != "pms" || got.Path != "" {
		t.Fatalf("workspace physical path leaked: %#v", got)
	}
}

func TestSanitizeRegistrationRejectsDuplicateWorkspaces(t *testing.T) {
	reg := protocol.RegisterRequest{
		DeviceID: "mac-1", DeviceName: "Mac",
		Workspaces: []protocol.Workspace{{Name: "pms"}, {Name: "pms"}},
	}
	if err := sanitizeRegistration(&reg); err == nil {
		t.Fatal("duplicate workspaces accepted")
	}
}

func TestSanitizeRegistrationBoundsMetadata(t *testing.T) {
	reg := protocol.RegisterRequest{DeviceID: strings.Repeat("x", 129), DeviceName: "Mac"}
	if err := sanitizeRegistration(&reg); err == nil {
		t.Fatal("oversized device id accepted")
	}

	reg = protocol.RegisterRequest{DeviceID: "mac", DeviceName: "Mac", Workspaces: make([]protocol.Workspace, 65)}
	for i := range reg.Workspaces {
		reg.Workspaces[i].Name = "w"
	}
	if err := sanitizeRegistration(&reg); err == nil {
		t.Fatal("too many workspaces accepted")
	}
}

func TestAgentHandlerExpiresMissingHeartbeat(t *testing.T) {
	store, err := authstore.Open("")
	if err != nil {
		t.Fatal(err)
	}
	code, _, err := store.CreateEnrollment(time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(2)
	server := httptest.NewServer(&AgentHandler{
		Registry:         registry,
		Auth:             store,
		HeartbeatTimeout: 100 * time.Millisecond,
	})
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	reg := protocol.RegisterRequest{
		EnrollmentCode: code,
		DeviceID:       "heartbeat-test",
		DeviceName:     "Heartbeat Test",
		Version:        "test",
		Workspaces:     []protocol.Workspace{{Name: "demo"}},
	}
	payload, err := json.Marshal(reg)
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.WriteJSON(protocol.Envelope{
		Type:     protocol.TypeRegister,
		DeviceID: reg.DeviceID,
		Payload:  payload,
	}); err != nil {
		t.Fatal(err)
	}

	var ack protocol.Envelope
	if err := ws.ReadJSON(&ack); err != nil {
		t.Fatal(err)
	}
	var registered protocol.RegisterResponse
	if err := json.Unmarshal(ack.Payload, &registered); err != nil {
		t.Fatal(err)
	}
	if !registered.Accepted {
		t.Fatalf("registration rejected: %s", registered.Message)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, ok := registry.Get(reg.DeviceID); !ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("device remained online after heartbeat timeout")
}
