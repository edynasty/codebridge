package manager

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/edynasty/codebridge/internal/auditlog"
	"github.com/edynasty/codebridge/internal/authstore"
	"github.com/edynasty/codebridge/internal/protocol"
	"github.com/gorilla/websocket"
)

const (
	maxAgentPayloadBytes         = 1024 * 1024
	defaultAgentHeartbeatTimeout = 75 * time.Second
)

type AgentHandler struct {
	Registry         *Registry
	Auth             *authstore.Store
	Audit            *auditlog.Logger
	HeartbeatTimeout time.Duration
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Local agents are native clients and do not send a browser Origin header.
		// Reject browser-originated WebSocket requests to reduce CSWSH exposure.
		return r.Header.Get("Origin") == ""
	},
}

func (h *AgentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	// Registration is small and unauthenticated. Keep this limit tight until
	// the device credential/enrollment code has been verified.
	ws.SetReadLimit(64 * 1024)
	_ = ws.SetReadDeadline(time.Now().Add(15 * time.Second))

	var first protocol.Envelope
	if err := ws.ReadJSON(&first); err != nil || first.Type != protocol.TypeRegister {
		_ = ws.WriteJSON(protocol.Envelope{Type: protocol.TypeError, Payload: mustJSON(map[string]string{"error": "first message must be register"})})
		return
	}
	var reg protocol.RegisterRequest
	if err := json.Unmarshal(first.Payload, &reg); err != nil {
		return
	}
	if err := sanitizeRegistration(&reg); err != nil {
		_ = ws.WriteJSON(protocol.Envelope{Type: protocol.TypeRegistered, Payload: mustJSON(protocol.RegisterResponse{Accepted: false, Message: err.Error()})})
		return
	}
	requestID := r.Header.Get(requestIDHeader)
	if h.Auth == nil {
		_ = ws.WriteJSON(protocol.Envelope{Type: protocol.TypeRegistered, Payload: mustJSON(protocol.RegisterResponse{Accepted: false, Message: "device authentication is not configured"})})
		return
	}

	issuedCredential := ""
	if h.Auth.VerifyDevice(reg.DeviceID, reg.DeviceCredential) {
		if err := h.Auth.UpdateDeviceName(reg.DeviceID, reg.DeviceName); err != nil {
			log.Printf("update device identity: %v", err)
		}
	} else {
		credential, err := h.Auth.EnrollDevice(reg.EnrollmentCode, reg.DeviceID, reg.DeviceName)
		if err != nil {
			_ = h.Audit.Log(auditlog.Event{Event: "device.enroll", RequestID: requestID, DeviceID: reg.DeviceID, Success: auditlog.Bool(false), ErrorKind: "invalid_enrollment"})
			_ = ws.WriteJSON(protocol.Envelope{Type: protocol.TypeRegistered, Payload: mustJSON(protocol.RegisterResponse{Accepted: false, Message: err.Error()})})
			return
		}
		issuedCredential = credential
		_ = h.Audit.Log(auditlog.Event{Event: "device.enroll", RequestID: requestID, DeviceID: reg.DeviceID, Success: auditlog.Bool(true)})
	}
	// The tenant account always comes from persisted device state, never from
	// agent-supplied input, so a device cannot relabel itself into another account.
	account, ok := h.Auth.DeviceAccount(reg.DeviceID)
	if !ok {
		_ = ws.WriteJSON(protocol.Envelope{Type: protocol.TypeRegistered, Payload: mustJSON(protocol.RegisterResponse{Accepted: false, Message: "device account binding not found"})})
		return
	}

	// Authenticated agent responses are bounded separately below.
	ws.SetReadLimit(maxAgentPayloadBytes + 64*1024)

	now := time.Now().UTC()
	conn := h.Registry.Put(Device{ID: reg.DeviceID, Name: reg.DeviceName, AccountID: account, Version: reg.Version, Online: true, ConnectedAt: now, LastSeen: now, Workspaces: reg.Workspaces, ToolPolicy: reg.ToolPolicy}, ws)
	defer h.Registry.Remove(reg.DeviceID, conn)
	// Once the connection is in the registry a concurrent tool call may write
	// to it under conn.mu, so the acknowledgment must take the same mutex.
	conn.mu.Lock()
	_ = ws.WriteJSON(protocol.Envelope{Type: protocol.TypeRegistered, Payload: mustJSON(protocol.RegisterResponse{Accepted: true, DeviceCredential: issuedCredential})})
	conn.mu.Unlock()
	heartbeatTimeout := h.HeartbeatTimeout
	if heartbeatTimeout <= 0 {
		heartbeatTimeout = defaultAgentHeartbeatTimeout
	}
	_ = ws.SetReadDeadline(time.Now().Add(heartbeatTimeout))
	log.Printf("agent online: %s (%s), workspaces=%d", reg.DeviceName, reg.DeviceID, len(reg.Workspaces))
	_ = h.Audit.Log(auditlog.Event{Event: "device.connect", RequestID: requestID, DeviceID: reg.DeviceID, Success: auditlog.Bool(true)})
	defer h.Audit.Log(auditlog.Event{Event: "device.disconnect", RequestID: requestID, DeviceID: reg.DeviceID, Success: auditlog.Bool(true)})

	for {
		var env protocol.Envelope
		if err := ws.ReadJSON(&env); err != nil {
			log.Printf("agent offline: %s (%s): %v", reg.DeviceName, reg.DeviceID, err)
			return
		}
		_ = ws.SetReadDeadline(time.Now().Add(heartbeatTimeout))
		h.Registry.Touch(reg.DeviceID)
		switch env.Type {
		case protocol.TypeHeartbeat:
			continue
		case protocol.TypeResponse:
			var resp protocol.AgentResponse
			if json.Unmarshal(env.Payload, &resp) == nil {
				if len(resp.Data) > maxAgentPayloadBytes {
					resp = protocol.AgentResponse{OK: false, Error: "agent response exceeded manager payload limit"}
				}
				conn.Deliver(env.RequestID, resp)
			}
		case protocol.TypeProgress:
			// Live progress from an in-flight tool (subagent event stream);
			// forwarded to the pending request's hook without completing it.
			conn.DeliverEvent(env.RequestID, env.Payload)
		}
	}
}

func sanitizeRegistration(reg *protocol.RegisterRequest) error {
	reg.DeviceID = strings.TrimSpace(reg.DeviceID)
	reg.DeviceName = strings.TrimSpace(reg.DeviceName)
	reg.Version = strings.TrimSpace(reg.Version)
	if reg.DeviceID == "" || reg.DeviceName == "" {
		return fmt.Errorf("device_id and device_name are required")
	}
	if len(reg.DeviceID) > 128 {
		return fmt.Errorf("device_id exceeds 128 bytes")
	}
	if len(reg.DeviceName) > 256 {
		return fmt.Errorf("device_name exceeds 256 bytes")
	}
	if len(reg.Version) > 64 {
		return fmt.Errorf("version exceeds 64 bytes")
	}
	if len(reg.Workspaces) > 64 {
		return fmt.Errorf("too many workspaces; maximum is 64")
	}
	seen := map[string]bool{}
	for i := range reg.Workspaces {
		name := strings.TrimSpace(reg.Workspaces[i].Name)
		if name == "" || len(name) > 128 {
			return fmt.Errorf("workspace name must be 1-128 bytes")
		}
		if seen[name] {
			return fmt.Errorf("duplicate workspace %q", name)
		}
		seen[name] = true
		reg.Workspaces[i].Name = name
		// Never trust or forward a physical path supplied by an agent.
		reg.Workspaces[i].Path = ""
	}
	return nil
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
