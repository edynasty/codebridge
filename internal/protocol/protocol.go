package protocol

import (
	"encoding/json"
	"time"
)

const (
	TypeRegister   = "register"
	TypeRegistered = "registered"
	TypeHeartbeat  = "heartbeat"
	TypeRequest    = "request"
	TypeResponse   = "response"
	// TypeProgress streams live subagent/in-flight events from agent to
	// manager while a request is still running; the manager forwards them
	// as MCP progress notifications.
	TypeProgress = "progress"
	// TypeRunEvent is an unsolicited durable runtime event. Unlike progress,
	// it is not tied to an outstanding request_id and may be replayed after a
	// reconnect using its per-run sequence number.
	TypeRunEvent = "run_event"
	// TypeRunHeads advertises the Client's durable event heads after connect.
	// TypeRunReplay asks the Client to replay the missing suffix for each run.
	TypeRunHeads  = "run_heads"
	TypeRunReplay = "run_replay"
	TypeError     = "error"
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

// ProgressEvent is the payload of a progress envelope: one live line from
// an in-flight tool execution (today: subagent event stream).
type ProgressEvent struct {
	RequestID string `json:"request_id"`
	Tool      string `json:"tool"`
	Event     string `json:"event"`
}

// RunEvent is the wire form of one ordered Runtime event. Seq is monotonic
// within a run. The manager uses it for idempotence and reconnect replay.
type RunEvent struct {
	RunID   string    `json:"run_id"`
	Seq     int64     `json:"seq"`
	At      time.Time `json:"at"`
	Source  string    `json:"source,omitempty"`
	Kind    string    `json:"kind"`
	Payload string    `json:"payload,omitempty"`
}

// RunHead is one Client-side durable journal head advertised after connect.
type RunHead struct {
	RunID   string `json:"run_id"`
	LastSeq int64  `json:"last_seq"`
	Status  string `json:"status,omitempty"`
}

type RunHeads struct {
	Runs []RunHead `json:"runs"`
}

// RunReplayCursor asks for (AfterSeq, ThroughSeq] of one run. ThroughSeq is
// the Client head observed during reconciliation, so a concurrently running
// task cannot make replay chase a moving target forever.
type RunReplayCursor struct {
	RunID      string `json:"run_id"`
	AfterSeq   int64  `json:"after_seq"`
	ThroughSeq int64  `json:"through_seq"`
}

type RunReplayRequest struct {
	Runs []RunReplayCursor `json:"runs"`
}
