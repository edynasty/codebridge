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
	// Writable is advertised by the local client from its own configuration;
	// a remote MCP caller can never enable it.
	Writable bool `json:"writable,omitempty"`
}

// CustomTool is one operator-defined MCP tool: a preset built-in tool call
// with fixed arguments and a stable public name. It lets a ChatGPT caller use
// a narrowed, self-describing method instead of raw device/workspace wiring.
type CustomTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Tool        string         `json:"tool"`
	Args        map[string]any `json:"args,omitempty"`
}

// ToolPolicy controls which MCP methods the caller sees. DisabledTools hides
// built-in tools; CustomTools adds preset wrappers. The policy travels with
// device registration and is applied by the Manager at tools/list time; the
// agent also rejects disabled tools as defense in depth.
type ToolPolicy struct {
	// EnabledTools is the explicit allowlist of built-in tools exposed to
	// this account; nil means the pi-style default core set. An entry "*"
	// exposes every built-in tool.
	EnabledTools  []string     `json:"enabled_tools,omitempty"`
	DisabledTools []string     `json:"disabled_tools,omitempty"`
	CustomTools   []CustomTool `json:"custom_tools,omitempty"`
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
	ToolPolicy       *ToolPolicy `json:"tool_policy,omitempty"`
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
