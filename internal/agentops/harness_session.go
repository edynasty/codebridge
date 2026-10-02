package agentops

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

const (
	SessionEntryMessage    = "message"
	SessionEntryThinking   = "thinking"
	SessionEntryToolCall   = "tool_call"
	SessionEntryToolResult = "tool_result"
	SessionEntryActivity   = "activity"
	SessionEntryRuntime    = "runtime"
	SessionEntryError      = "error"
)

// HarnessSessionRef is safe to surface upstream. Locator is deliberately
// excluded from JSON because it is a physical local path used only by the
// Client adapter.
type HarnessSessionRef struct {
	Harness   string    `json:"harness"`
	ID        string    `json:"id"`
	Title     string    `json:"title,omitempty"`
	Model     string    `json:"model,omitempty"`
	Thinking  string    `json:"thinking,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`

	Locator string `json:"-"`
	root    string
}

// SessionEntry is the stable presentation projection shared by OMP, Codex and
// OpenCode. It represents only data explicitly exposed by the harness.
type SessionEntry struct {
	SourceSeq int64           `json:"source_seq"`
	At        time.Time       `json:"at,omitempty"`
	Kind      string          `json:"kind"`
	Role      string          `json:"role,omitempty"`
	Text      string          `json:"text,omitempty"`
	ToolName  string          `json:"tool_name,omitempty"`
	ToolCall  string          `json:"tool_call_id,omitempty"`
	Status    string          `json:"status,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	RawType   string          `json:"raw_type,omitempty"`
}

// HarnessRawRecord preserves one harness-native record exactly as the local
// adapter observed it. Payload deliberately has no JSON representation:
// Runtime may index/display raw harness data locally, but an upstream API must
// opt in to an explicit projection instead of accidentally serializing native
// records that can contain local paths or other machine-specific details.
type HarnessRawRecord struct {
	SourceSeq int64           `json:"source_seq"`
	At        time.Time       `json:"at,omitempty"`
	RawType   string          `json:"raw_type,omitempty"`
	Payload   json.RawMessage `json:"-"`
}

// HarnessLiveEvent is the adapter result for one live harness record. Record
// keeps the native bytes for durable/local inspection; Entries is the
// best-effort stable presentation projection. One native record may project
// to zero or several entries.
type HarnessLiveEvent struct {
	Record  HarnessRawRecord `json:"-"`
	Entries []SessionEntry   `json:"entries,omitempty"`
}

type SessionListRequest struct {
	Root  string
	Limit int
}

type SessionDiscoverRequest struct {
	Root      string
	Task      string
	StartedAt time.Time
	// NativeID is an optional harness-native stable session/thread ID learned
	// from live events. Adapters should prefer it over heuristic matching.
	NativeID string
}

type SessionReadRequest struct {
	After int64
	Limit int
}

type SessionPage struct {
	Session    HarnessSessionRef  `json:"session"`
	Entries    []SessionEntry     `json:"entries"`
	RawRecords []HarnessRawRecord `json:"-"`
	NextCursor int64              `json:"next_cursor"`
	EOF        bool               `json:"eof"`
}

// HarnessSessionAdapter bridges one headless agent's live JSON output and
// durable session store into CodeBridge Runtime. Implementations may expose
// different native fields; only the presentation projection is shared
// upstream. Native records remain available locally through HarnessLiveEvent
// and SessionPage.RawRecords.
type HarnessSessionAdapter interface {
	Harness() string
	List(context.Context, SessionListRequest) ([]HarnessSessionRef, error)
	Discover(context.Context, SessionDiscoverRequest) (HarnessSessionRef, error)
	Read(context.Context, HarnessSessionRef, SessionReadRequest) (SessionPage, error)
	NormalizeLive([]byte) (HarnessLiveEvent, error)
}

var ErrHarnessSessionNotFound = errors.New("harness session not found")
