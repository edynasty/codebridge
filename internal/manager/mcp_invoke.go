package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/edynasty/codebridge/internal/auditlog"
	"github.com/edynasty/codebridge/internal/mcpcallstore"
	"github.com/edynasty/codebridge/internal/protocol"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const requestIDHeader = "X-CodeBridge-Request-ID"

func (t *ToolService) invoke(req *mcp.CallToolRequest, tool, deviceID, workspace string, fn func() (*mcp.CallToolResult, any, error)) (*mcp.CallToolResult, any, error) {
	return t.invokeArgs(req, tool, deviceID, workspace, nil, fn)
}

// invokeArgs is invoke with the tool arguments captured for the call store.
func (t *ToolService) invokeArgs(req *mcp.CallToolRequest, tool, deviceID, workspace string, args map[string]any, fn func() (*mcp.CallToolResult, any, error)) (*mcp.CallToolResult, any, error) {
	start := time.Now()
	result, out, err := fn()

	requestID := ""
	actorID := ""
	if req != nil {
		requestID = strings.TrimSpace(req.Extra.Header.Get(requestIDHeader))
		if req.Extra.TokenInfo != nil {
			actorID = req.Extra.TokenInfo.UserID
		}
	}
	if requestID == "" {
		requestID = auditlog.NewRequestID()
	}

	// Dual write: the JSONL audit log is the append-only source of truth;
	// the SQLite store backs filtering/paging in the admin console.
	if t.CallStore != nil {
		argsJSON := ""
		if args != nil {
			if b, marshalErr := json.Marshal(args); marshalErr == nil {
				argsJSON = string(b)
			}
		}
		if insertErr := t.CallStore.Insert(mcpcallstore.Call{
			ArgsJSON:   argsJSON,
			Time:       time.Now().UTC(),
			RequestID:  requestID,
			ActorID:    actorID,
			Tool:       tool,
			DeviceID:   deviceID,
			Workspace:  workspace,
			Success:    err == nil,
			DurationMS: time.Since(start).Milliseconds(),
			ErrorKind:  auditErrorKind(err),
		}); insertErr != nil {
			log.Printf("mcp call store insert: %v", insertErr)
		}
	}
	_ = t.Audit.Log(auditlog.Event{
		Event:      "mcp.tool",
		RequestID:  requestID,
		ActorID:    actorID,
		Tool:       tool,
		DeviceID:   deviceID,
		Workspace:  workspace,
		Success:    auditlog.Bool(err == nil),
		DurationMS: time.Since(start).Milliseconds(),
		ErrorKind:  auditErrorKind(err),
	})
	return result, out, err
}

// summarizeProgress renders one progress envelope as a short message for
// the MCP progress notification (tool calls and text snippets, bounded).
func summarizeProgress(payload json.RawMessage) string {
	var ev struct {
		Tool  string `json:"tool"`
		Event string `json:"event"`
	}
	if json.Unmarshal(payload, &ev) != nil || ev.Event == "" {
		return "progress"
	}
	line := strings.TrimSpace(ev.Event)
	var parsed struct {
		Type string `json:"type"`
		Part struct {
			Type string `json:"type"`
			Text string `json:"text"`
			Tool string `json:"tool"`
		} `json:"part"`
	}
	if json.Unmarshal([]byte(line), &parsed) == nil {
		switch parsed.Type {
		case "tool_use", "tool_result":
			name := parsed.Part.Tool
			if name == "" {
				name = parsed.Type
			}
			return "tool: " + name
		case "text":
			if parsed.Part.Text != "" {
				t := parsed.Part.Text
				if len(t) > 120 {
					t = t[:120] + "…"
				}
				return strings.ReplaceAll(t, "\n", " ")
			}
		case "step_start":
			return "step started"
		case "step_finish":
			return "step finished"
		}
	}
	if len(line) > 120 {
		line = line[:120] + "…"
	}
	return line
}

func auditErrorKind(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case strings.Contains(err.Error(), "offline or unknown"):
		return "device_unavailable"
	case strings.Contains(err.Error(), "required"), strings.Contains(err.Error(), "unknown workspace"):
		return "invalid_request"
	default:
		return "tool_error"
	}
}

func (t *ToolService) forward(ctx context.Context, account, deviceID, workspace, tool string, args map[string]any) (*mcp.CallToolResult, any, error) {
	return t.forwardProgress(ctx, nil, account, deviceID, workspace, tool, args)
}

// forwardProgress is forward with live progress: onEvent is invoked for
// every progress envelope the agent streams while the call is running.
func (t *ToolService) forwardProgress(ctx context.Context, req *mcp.CallToolRequest, account, deviceID, workspace, tool string, args map[string]any) (*mcp.CallToolResult, any, error) {
	if deviceID == "" || workspace == "" {
		return nil, nil, fmt.Errorf("device_id and workspace are required")
	}
	var onEvent func(json.RawMessage)
	if req != nil && req.Session != nil {
		token := req.Params.GetProgressToken()
		if token != nil {
			session := req.Session
			var seq int64
			onEvent = func(payload json.RawMessage) {
				seq++
				msg := summarizeProgress(payload)
				_ = session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
					ProgressToken: token,
					Progress:      float64(seq),
					Message:       msg,
				})
			}
		}
	}
	raw, err := t.Registry.CallProgress(ctx, account, deviceID, protocolRequest(tool, workspace, args), onEvent)
	if err != nil {
		return nil, nil, err
	}
	var out any
	if len(raw) == 0 {
		out = map[string]any{"ok": true}
	} else if err := json.Unmarshal(raw, &out); err != nil {
		out = string(raw)
	}
	out, err = toolOutputPayload(tool, stringArg(args, "path", ""), out)
	if err != nil {
		return nil, nil, err
	}
	return textResult(out)
}

func protocolRequest(tool, workspace string, args map[string]any) protocol.AgentRequest {
	if args == nil {
		args = map[string]any{}
	}
	return protocol.AgentRequest{Tool: tool, Workspace: workspace, Args: args}
}
