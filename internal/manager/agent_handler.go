package manager

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/edynasty/codebridge/internal/protocol"
	"github.com/gorilla/websocket"
)

type AgentHandler struct {
	Registry    *Registry
	EnrollToken string
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
	if h.EnrollToken != "" && reg.EnrollToken != h.EnrollToken {
		_ = ws.WriteJSON(protocol.Envelope{Type: protocol.TypeRegistered, Payload: mustJSON(protocol.RegisterResponse{Accepted: false, Message: "invalid enrollment token"})})
		return
	}
	reg.DeviceID = strings.TrimSpace(reg.DeviceID)
	reg.DeviceName = strings.TrimSpace(reg.DeviceName)
	if reg.DeviceID == "" || reg.DeviceName == "" {
		_ = ws.WriteJSON(protocol.Envelope{Type: protocol.TypeRegistered, Payload: mustJSON(protocol.RegisterResponse{Accepted: false, Message: "device_id and device_name are required"})})
		return
	}
	now := time.Now().UTC()
	conn := h.Registry.Put(Device{ID: reg.DeviceID, Name: reg.DeviceName, Version: reg.Version, Online: true, ConnectedAt: now, LastSeen: now, Workspaces: reg.Workspaces}, ws)
	defer h.Registry.Remove(reg.DeviceID, conn)
	_ = ws.WriteJSON(protocol.Envelope{Type: protocol.TypeRegistered, Payload: mustJSON(protocol.RegisterResponse{Accepted: true})})
	_ = ws.SetReadDeadline(time.Time{})
	log.Printf("agent online: %s (%s), workspaces=%d", reg.DeviceName, reg.DeviceID, len(reg.Workspaces))

	for {
		var env protocol.Envelope
		if err := ws.ReadJSON(&env); err != nil {
			log.Printf("agent offline: %s (%s): %v", reg.DeviceName, reg.DeviceID, err)
			return
		}
		h.Registry.Touch(reg.DeviceID)
		switch env.Type {
		case protocol.TypeHeartbeat:
			continue
		case protocol.TypeResponse:
			var resp protocol.AgentResponse
			if json.Unmarshal(env.Payload, &resp) == nil {
				conn.Deliver(env.RequestID, resp)
			}
		}
	}
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
