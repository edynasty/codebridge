package protocol

import "encoding/json"

const (
	TypeRegister   = "register"
	TypeRegistered = "registered"
	TypeHeartbeat  = "heartbeat"
	TypeRequest    = "request"
	TypeResponse   = "response"
	TypeError      = "error"
)

type Workspace struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

type Envelope struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	DeviceID  string          `json:"device_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type RegisterRequest struct {
	EnrollmentCode   string      `json:"enrollment_code,omitempty"`
	DeviceCredential string      `json:"device_credential,omitempty"`
	DeviceID         string      `json:"device_id"`
	DeviceName       string      `json:"device_name"`
	Version          string      `json:"version"`
	Workspaces       []Workspace `json:"workspaces"`
}

type RegisterResponse struct {
	Accepted         bool   `json:"accepted"`
	Message          string `json:"message,omitempty"`
	DeviceCredential string `json:"device_credential,omitempty"`
}

type AgentRequest struct {
	Tool      string         `json:"tool"`
	Workspace string         `json:"workspace,omitempty"`
	Args      map[string]any `json:"args,omitempty"`
}

type AgentResponse struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}
