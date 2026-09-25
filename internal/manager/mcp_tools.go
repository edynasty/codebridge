package manager

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/edynasty/codebridge/internal/auditlog"
	"github.com/edynasty/codebridge/internal/mcpcallstore"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolService installs the MCP tool surface onto servers.
type ToolService struct {
	Registry    *Registry
	Accounts    *AccountResolver
	OAuthScopes []string
	Audit       *auditlog.Logger
	// CallStore persists MCP tool calls for the admin console (optional;
	// nil disables the query API, the JSONL audit log still records them).
	CallStore *mcpcallstore.Store
}

func (t *ToolService) readOnlyTool(name, description string) *mcp.Tool {
	return t.annotatedTool(name, description, true, false, name)
}

// writeTool declares a mutating tool. Write tools are only routed to
// workspaces the local client explicitly advertised as writable.
func (t *ToolService) writeTool(name, description string) *mcp.Tool {
	return t.annotatedTool(name, description, false, true, name)
}

// annotatedTool builds a tool descriptor. outputSource names the built-in tool
// whose output schema this tool returns: it is the tool's own name, except for
// custom wrappers, which report the output of the built-in they forward to.
func (t *ToolService) annotatedTool(name, description string, readOnly, destructive bool, outputSource string) *mcp.Tool {
	outputSchema, ok := toolOutputSchemas[outputSource]
	if !ok {
		panic("missing output schema for tool: " + outputSource)
	}
	tool := &mcp.Tool{
		Name:         name,
		Description:  description,
		OutputSchema: outputSchema,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    readOnly,
			DestructiveHint: boolPtr(destructive),
			OpenWorldHint:   boolPtr(false),
		},
	}
	if len(t.OAuthScopes) > 0 {
		scopes := append([]string(nil), t.OAuthScopes...)
		// The upstream Go MCP SDK v1.8.0 exposes arbitrary tool metadata but
		// does not yet have [OI]'s securitySchemes extension as a top-level
		// Go field. ChatGPT supports this _meta mirror for compatibility.
		tool.Meta = mcp.Meta{
			"securitySchemes": []any{
				map[string]any{"type": "oauth2", "scopes": scopes},
			},
		}
	}
	return tool
}

func boolPtr(b bool) *bool { return &b }

func textResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, v, nil
}

// The MCP surface is deliberately pi-shaped (badlogic/pi-mono): a small set
// of composable primitives AI models already know — read, write, edit, bash,
// list — plus device/workspace discovery. Niche helpers (symbol search,
// dependency graphs, git wrappers) were removed: bash covers them, and
// operators can define custom wrapper tools when they want a narrowed
// surface for a specific workflow.

type DeviceInput struct {
	DeviceID string `json:"device_id" jsonschema:"ID of the connected device"`
}

type ReadInput struct {
	DeviceID  string `json:"device_id" jsonschema:"ID of the connected device"`
	Workspace string `json:"workspace" jsonschema:"Workspace name exposed by the local agent"`
	Path      string `json:"path" jsonschema:"Workspace-relative file path"`
	MaxBytes  int    `json:"max_bytes,omitempty" jsonschema:"Optional byte cap; default 256 KiB"`
}

type EditInput struct {
	DeviceID  string `json:"device_id" jsonschema:"ID of the connected device"`
	Workspace string `json:"workspace" jsonschema:"Workspace name exposed by the local agent"`
	Path      string `json:"path" jsonschema:"Workspace-relative file path"`
	// Write-only replaces the whole file content.
	Content string `json:"content,omitempty" jsonschema:"Full new file content (write mode)"`
	// Edit replaces exactly one occurrence of OldText with NewText.
	OldText string `json:"old_text,omitempty" jsonschema:"Exact current text to replace; must match once"`
	NewText string `json:"new_text,omitempty" jsonschema:"Replacement text"`
	// Preview/confirm mirror the two-step apply flow.
	Preview bool `json:"preview,omitempty" jsonschema:"Return the proposed diff without writing"`
	Confirm bool `json:"confirm,omitempty" jsonschema:"Required true to actually write"`
}

type ListInput struct {
	DeviceID  string `json:"device_id" jsonschema:"ID of the connected device"`
	Workspace string `json:"workspace" jsonschema:"Workspace name exposed by the local agent"`
	Path      string `json:"path,omitempty" jsonschema:"Optional workspace-relative directory; default ."`
}

// PatchEdit is one all-or-nothing edit inside apply_patch: replace (old_text and
// new_text), create (new_text only) or delete (old_text only).
type PatchEdit struct {
	Path    string `json:"path" jsonschema:"Workspace-relative file path"`
	OldText string `json:"old_text,omitempty" jsonschema:"Exact current text; must match once (replace) or be the whole file (delete)"`
	NewText string `json:"new_text,omitempty" jsonschema:"Replacement text; empty deletes the file"`
}

type ApplyPatchInput struct {
	DeviceID  string      `json:"device_id" jsonschema:"ID of the connected device"`
	Workspace string      `json:"workspace" jsonschema:"Workspace name exposed by the local agent"`
	Edits     []PatchEdit `json:"edits" jsonschema:"All-or-nothing edits applied together"`
	Preview   bool        `json:"preview,omitempty" jsonschema:"Return the proposed diff without writing"`
	Confirm   bool        `json:"confirm,omitempty" jsonschema:"Required true to actually write"`
}

type RollbackPatchInput struct {
	DeviceID     string `json:"device_id" jsonschema:"ID of the connected device"`
	Workspace    string `json:"workspace" jsonschema:"Workspace name exposed by the local agent"`
	CheckpointID string `json:"checkpoint_id" jsonschema:"Checkpoint ID returned by apply_patch, write or edit"`
}

type BashInput struct {
	DeviceID  string `json:"device_id" jsonschema:"ID of the connected device"`
	Workspace string `json:"workspace" jsonschema:"Workspace name exposed by the local agent"`
	Command   string `json:"command" jsonschema:"Shell command to run in the workspace root"`
}

type AgentInput struct {
	DeviceID       string `json:"device_id" jsonschema:"ID of the connected device"`
	Workspace      string `json:"workspace" jsonschema:"Workspace the subagent works in"`
	Task           string `json:"task" jsonschema:"Self-contained task description for the subagent"`
	Client         string `json:"client,omitempty" jsonschema:"Subagent harness: omp (default), opencode or codex"`
	Agent          string `json:"agent,omitempty" jsonschema:"opencode agent name, codex profile, or a subagent profile configured on the client"`
	Model          string `json:"model,omitempty" jsonschema:"Model override, provider/model format"`
	Thinking       string `json:"thinking,omitempty" jsonschema:"Reasoning effort: off, minimal, low, medium, high, xhigh, max"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"Optional wall-clock budget in seconds; 0 or omitted means no limit"`
}

type PermissionGrantInput struct {
	DeviceID  string `json:"device_id" jsonschema:"ID of the connected device the request belongs to"`
	Workspace string `json:"workspace" jsonschema:"Workspace the request belongs to"`
	RequestID string `json:"request_id" jsonschema:"ID of the pending permission request reported by a blocked tool call"`
	// Once approves this single execution; always persists a rule; deny rejects.
	Decision string `json:"decision" jsonschema:"once, always, or deny"`
}

// toolTable is the single source of truth for the built-in surface (also
// used by the per-request policy server).
var toolTable = map[string]func(t *ToolService, server *mcp.Server){
	"list_devices": func(t *ToolService, server *mcp.Server) {
		mcp.AddTool(server, t.readOnlyTool("list_devices", "List local CodeBridge devices currently connected to this manager."), func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, any, error) {
			account := t.accountFor(req)
			return t.invoke(req, "list_devices", "", "", func() (*mcp.CallToolResult, any, error) {
				devices := t.Registry.ListAccount(account)
				return textResult(map[string]any{"devices": devices, "count": len(devices)})
			})
		})
	},

	"list_workspaces": func(t *ToolService, server *mcp.Server) {
		mcp.AddTool(server, t.readOnlyTool("list_workspaces", "List source-code workspaces exposed by one connected device."), func(ctx context.Context, req *mcp.CallToolRequest, in DeviceInput) (*mcp.CallToolResult, any, error) {
			account := t.accountFor(req)
			return t.invoke(req, "list_workspaces", in.DeviceID, "", func() (*mcp.CallToolResult, any, error) {
				v, err := t.Registry.Workspaces(account, in.DeviceID)
				if err != nil {
					return nil, nil, err
				}
				return textResult(map[string]any{"workspaces": v, "count": len(v)})
			})
		})
	},

	"list": func(t *ToolService, server *mcp.Server) {
		mcp.AddTool(server, t.readOnlyTool("list", "List files and directories inside a workspace, like ls."), func(ctx context.Context, req *mcp.CallToolRequest, in ListInput) (*mcp.CallToolResult, any, error) {
			account := t.accountFor(req)
			path := in.Path
			if path == "" {
				path = "."
			}
			return t.invokeArgs(req, "list", in.DeviceID, in.Workspace, map[string]any{"path": path}, func() (*mcp.CallToolResult, any, error) {
				return t.forward(ctx, account, in.DeviceID, in.Workspace, "list", map[string]any{"path": path})
			})
		})
	},

	"read": func(t *ToolService, server *mcp.Server) {
		mcp.AddTool(server, t.readOnlyTool("read", "Read a text file from a workspace (workspace-relative path, bounded)."), func(ctx context.Context, req *mcp.CallToolRequest, in ReadInput) (*mcp.CallToolResult, any, error) {
			account := t.accountFor(req)
			maxBytes := in.MaxBytes
			if maxBytes <= 0 {
				maxBytes = 256 * 1024
			}
			return t.invokeArgs(req, "read", in.DeviceID, in.Workspace, map[string]any{"path": in.Path, "max_bytes": maxBytes}, func() (*mcp.CallToolResult, any, error) {
				return t.forward(ctx, account, in.DeviceID, in.Workspace, "read", map[string]any{"path": in.Path, "max_bytes": maxBytes})
			})
		})
	},

	"edit": func(t *ToolService, server *mcp.Server) {
		mcp.AddTool(server, t.writeTool("edit", "Edit a file: replace one exact occurrence of old_text with new_text (all-or-nothing, preview then confirm)."), func(ctx context.Context, req *mcp.CallToolRequest, in EditInput) (*mcp.CallToolResult, any, error) {
			account := t.accountFor(req)
			return t.invokeArgs(req, "edit", in.DeviceID, in.Workspace, map[string]any{
				"path": in.Path, "old_text": in.OldText, "new_text": in.NewText,
				"preview": in.Preview, "confirm": in.Confirm,
			}, func() (*mcp.CallToolResult, any, error) {
				return t.forward(ctx, account, in.DeviceID, in.Workspace, "edit", map[string]any{
					"path": in.Path, "old_text": in.OldText, "new_text": in.NewText,
					"preview": in.Preview, "confirm": in.Confirm,
				})
			})
		})
	},

	"write": func(t *ToolService, server *mcp.Server) {
		mcp.AddTool(server, t.writeTool("write", "Write a file's full content (create or overwrite; preview then confirm). Sensitive paths and binary content are rejected."), func(ctx context.Context, req *mcp.CallToolRequest, in EditInput) (*mcp.CallToolResult, any, error) {
			account := t.accountFor(req)
			return t.invokeArgs(req, "write", in.DeviceID, in.Workspace, map[string]any{
				"path": in.Path, "content": in.Content,
				"preview": in.Preview, "confirm": in.Confirm,
			}, func() (*mcp.CallToolResult, any, error) {
				return t.forward(ctx, account, in.DeviceID, in.Workspace, "write", map[string]any{
					"path": in.Path, "content": in.Content,
					"preview": in.Preview, "confirm": in.Confirm,
				})
			})
		})
	},

	"apply_patch": func(t *ToolService, server *mcp.Server) {
		mcp.AddTool(server, t.writeTool("apply_patch", "Apply several all-or-nothing file edits in one call (create/replace/delete; preview then confirm). Every apply creates a local checkpoint that rollback_patch can undo. Sensitive paths and binary content are rejected."), func(ctx context.Context, req *mcp.CallToolRequest, in ApplyPatchInput) (*mcp.CallToolResult, any, error) {
			account := t.accountFor(req)
			edits := make([]map[string]any, 0, len(in.Edits))
			for _, edit := range in.Edits {
				edits = append(edits, map[string]any{"path": edit.Path, "old_text": edit.OldText, "new_text": edit.NewText})
			}
			args := map[string]any{"edits": edits, "preview": in.Preview, "confirm": in.Confirm}
			return t.invokeArgs(req, "apply_patch", in.DeviceID, in.Workspace, args, func() (*mcp.CallToolResult, any, error) {
				return t.forward(ctx, account, in.DeviceID, in.Workspace, "apply_patch", args)
			})
		})
	},

	"rollback_patch": func(t *ToolService, server *mcp.Server) {
		mcp.AddTool(server, t.writeTool("rollback_patch", "Undo one earlier write by restoring the files it touched from its local checkpoint. Checkpoints are kept on the device, bounded to the most recent 20."), func(ctx context.Context, req *mcp.CallToolRequest, in RollbackPatchInput) (*mcp.CallToolResult, any, error) {
			account := t.accountFor(req)
			args := map[string]any{"checkpoint_id": in.CheckpointID}
			return t.invokeArgs(req, "rollback_patch", in.DeviceID, in.Workspace, args, func() (*mcp.CallToolResult, any, error) {
				return t.forward(ctx, account, in.DeviceID, in.Workspace, "rollback_patch", args)
			})
		})
	},

	"bash": func(t *ToolService, server *mcp.Server) {
		mcp.AddTool(server, t.writeTool("bash", "Run a shell command with the workspace as working directory (like pi's bash). Commands need local permission: the first call returns a permission request id; the operator then calls permission_grant (decision once/always/deny). 30s timeout, bounded output, no interactive input."), func(ctx context.Context, req *mcp.CallToolRequest, in BashInput) (*mcp.CallToolResult, any, error) {
			account := t.accountFor(req)
			return t.invokeArgs(req, "bash", in.DeviceID, in.Workspace, map[string]any{"command": in.Command}, func() (*mcp.CallToolResult, any, error) {
				return t.forward(ctx, account, in.DeviceID, in.Workspace, "bash", map[string]any{"command": in.Command})
			})
		})
	},

	"agents_list": func(t *ToolService, server *mcp.Server) {
		mcp.AddTool(server, t.readOnlyTool("agents_list", "List the subagent profiles available on a device, with each profile's description, harness, model, reasoning effort and timeout. Call this before the agent tool to pick the right profile for the task."), func(ctx context.Context, req *mcp.CallToolRequest, in DeviceInput) (*mcp.CallToolResult, any, error) {
			account := t.accountFor(req)
			return t.invoke(req, "agents_list", in.DeviceID, "", func() (*mcp.CallToolResult, any, error) {
				return t.forward(ctx, account, in.DeviceID, "__catalog__", "agents_list", nil)
			})
		})
	},

	"agent": func(t *ToolService, server *mcp.Server) {
		mcp.AddTool(server, t.writeTool("agent", "Delegate a self-contained task to a local coding-agent subagent that works autonomously inside one workspace and returns its final output. Usage: call agents_list first, pick a profile, pass its name as the agent parameter, and write a complete task description (the subagent shares no conversation context with you). The client must allow the harness with a permission rule (allow agent omp / allow agent opencode / allow agent codex); the first call otherwise returns a permission request id to resolve with permission_grant."), func(ctx context.Context, req *mcp.CallToolRequest, in AgentInput) (*mcp.CallToolResult, any, error) {
			account := t.accountFor(req)
			return t.invokeArgs(req, "agent", in.DeviceID, in.Workspace, map[string]any{
				"task": in.Task, "client": in.Client, "agent": in.Agent,
				"model": in.Model, "thinking": in.Thinking,
				"timeout_seconds": in.TimeoutSeconds,
			}, func() (*mcp.CallToolResult, any, error) {
				// forwardProgress streams live subagent events to the MCP
				// client as progress notifications while the call runs.
				return t.forwardProgress(ctx, req, account, in.DeviceID, in.Workspace, "agent", map[string]any{
					"task": in.Task, "client": in.Client, "agent": in.Agent,
					"model": in.Model, "thinking": in.Thinking,
					"timeout_seconds": in.TimeoutSeconds,
				})
			})
		})
	},

	"permission_grant": func(t *ToolService, server *mcp.Server) {
		mcp.AddTool(server, t.readOnlyTool("permission_grant", "Approve or reject a pending permission request reported by a blocked bash call: decision once (this execution), always (persist a rule for this command prefix), or deny."), func(ctx context.Context, req *mcp.CallToolRequest, in PermissionGrantInput) (*mcp.CallToolResult, any, error) {
			account := t.accountFor(req)
			return t.invoke(req, "permission_grant", "", "", func() (*mcp.CallToolResult, any, error) {
				if in.DeviceID == "" && in.Workspace == "" {
					return nil, nil, fmt.Errorf("device_id and workspace are required")
				}
				return t.forward(ctx, account, in.DeviceID, in.Workspace, "permission_grant", map[string]any{"request_id": in.RequestID, "decision": in.Decision})
			})
		})
	},
}

// Register installs every built-in tool onto the server.
func (t *ToolService) Register(server *mcp.Server) {
	for _, name := range builtinToolNames {
		t.RegisterOne(server, name)
	}
}

// RegisterOne installs a single built-in tool by name. Unknown names are a
// programming error.
func (t *ToolService) RegisterOne(server *mcp.Server, name string) {
	fn, ok := toolTable[name]
	if !ok {
		panic("unknown built-in tool: " + name)
	}
	fn(t, server)
}
