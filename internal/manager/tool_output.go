package manager

import "fmt"

// Output schemas for the built-in MCP tools.
//
// The MCP specification requires structuredContent to be a JSON object, and a
// tool that declares an output schema MUST return structured results that
// conform to it. ChatGPT reads those schemas when it builds the Tool
// namespace, so every payload the manager emits is validated against the
// schema declared here.
//
// Tools that naturally return a list wrap it in an object envelope
// ({"devices": [...], "count": n}); toolOutputPayload applies that wrapping,
// and the schemas below describe the wrapped shape.

// errMalformedPayload reports a client response that cannot conform to the
// tool's declared output schema.
func errMalformedPayload(tool string) error {
	return fmt.Errorf("%s: client returned a payload that does not match the tool's output schema", tool)
}

func schemaString(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func schemaBool(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func schemaInt(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

// schemaObject builds an object schema; properties may be nil for a free-form
// or opaque object.
func schemaObject(description string, properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{"type": "object"}
	if description != "" {
		schema["description"] = description
	}
	if properties != nil {
		schema["properties"] = properties
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func schemaArray(items map[string]any, description string) map[string]any {
	schema := map[string]any{"type": "array", "items": items}
	if description != "" {
		schema["description"] = description
	}
	return schema
}

func schemaStringArray(description string) map[string]any {
	return schemaArray(map[string]any{"type": "string"}, description)
}

// workspaceOutputSchema mirrors protocol.Workspace as the local client reports
// it: path is only present when the client advertises one, and writable is
// omitted for read-only workspaces.
var workspaceOutputSchema = schemaObject(
	"One workspace exposed by a device",
	map[string]any{
		"name":     schemaString("Workspace name, as passed to other tool calls"),
		"path":     schemaString("Local path the workspace maps to, when the client advertises it"),
		"writable": schemaBool("True when the client enabled write mode for this workspace"),
	},
	"name",
)

var customToolOutputSchema = schemaObject(
	"One operator-defined tool wrapper exposed by the client",
	map[string]any{
		"name":        schemaString("Public tool name of the wrapper"),
		"description": schemaString("Description shown to callers"),
		"tool":        schemaString("Built-in tool the wrapper forwards to"),
		"args":        schemaObject("Fixed arguments the wrapper pins", nil),
	},
	"name", "tool",
)

var toolPolicyOutputSchema = schemaObject(
	"Tool policy the client advertised at registration",
	map[string]any{
		"enabled_tools":  schemaStringArray("Built-in tools the client allows; \"*\" means all"),
		"disabled_tools": schemaStringArray("Built-in tools the client hides"),
		"custom_tools":   schemaArray(customToolOutputSchema, "Custom wrappers the client defines"),
	},
)

var deviceOutputSchema = schemaObject(
	"One connected local CodeBridge device",
	map[string]any{
		"id":           schemaString("Device ID to pass to other tool calls"),
		"name":         schemaString("Human-readable device name"),
		"account_id":   schemaString("Account the device is registered to"),
		"version":      schemaString("Client build version reported at registration"),
		"online":       schemaBool("Connection state at the time of the call"),
		"connected_at": schemaString("RFC 3339 timestamp of the current connection"),
		"last_seen":    schemaString("RFC 3339 timestamp of the last heartbeat"),
		"workspaces":   schemaArray(workspaceOutputSchema, "Workspaces the device exposes"),
		"tool_policy":  toolPolicyOutputSchema,
	},
	"id", "name", "account_id", "version", "online", "connected_at", "last_seen", "workspaces",
)

var dirEntryOutputSchema = schemaObject(
	"One directory entry",
	map[string]any{
		"name": schemaString("File or directory name"),
		"path": schemaString("Workspace-relative path"),
		"type": map[string]any{
			"type":        "string",
			"description": "Entry kind",
			"enum":        []string{"file", "dir", "symlink"},
		},
		"size": schemaInt("Size in bytes; omitted for directories and symlinks"),
	},
	"name", "path", "type",
)

var patchSummaryOutputSchema = schemaObject(
	"One file touched by a write, edit or apply_patch call",
	map[string]any{
		"path":   schemaString("Workspace-relative file path"),
		"action": schemaString("create, replace or delete"),
	},
	"path", "action",
)

// patchOutputSchema covers write, edit and apply_patch, which all return the
// same result shape.
var patchOutputSchema = schemaObject(
	"Result of a write, edit or apply_patch call",
	map[string]any{
		"applied":       schemaBool("True when the change was written; false for a preview"),
		"preview":       schemaBool("True when this result only previewed the change"),
		"diff":          schemaString("Unified diff of the change"),
		"files":         schemaArray(patchSummaryOutputSchema, "Every file the call touched"),
		"checkpoint_id": schemaString("Pass to rollback_patch to undo this write"),
		"message":       schemaString("Human-readable outcome, including how to roll back"),
	},
	"applied", "files",
)

var rollbackOutputSchema = schemaObject(
	"Result of a rollback_patch call",
	map[string]any{
		"rolled_back":    schemaBool("True when the checkpoint was restored"),
		"checkpoint_id":  schemaString("Checkpoint that was restored"),
		"restored_files": schemaStringArray("Paths restored from the checkpoint"),
		"removed_files":  schemaStringArray("Paths deleted because they did not exist before the checkpoint"),
	},
	"rolled_back", "checkpoint_id", "restored_files",
)

var bashOutputSchema = schemaObject(
	"Result of a bash command",
	map[string]any{
		"command":   schemaString("Command that ran"),
		"output":    schemaString("Combined stdout and stderr"),
		"truncated": schemaBool("True when output hit the size cap"),
		"error":     schemaString("Set when the command failed or timed out"),
		"exit_code": schemaInt("Exit status; present when the command failed"),
	},
	"command", "output", "truncated",
)

var agentCatalogEntryOutputSchema = schemaObject(
	"One subagent profile configured on the device",
	map[string]any{
		"name":            schemaString("Profile name; pass it as the agent parameter of the agent tool"),
		"description":     schemaString("What the profile is for"),
		"client":          schemaString("Harness the profile runs: omp, opencode or codex"),
		"agent":           schemaString("opencode agent name or codex profile; not used by omp"),
		"model":           schemaString("Model override in provider/model form"),
		"thinking":        schemaString("Reasoning effort"),
		"timeout_seconds": schemaInt("Wall-clock budget; omitted means no limit"),
		"tab":             schemaString("Configuration-UI group the profile belongs to"),
	},
	"name",
)

var agentOutputSchema = schemaObject(
	"Result of one subagent run",
	map[string]any{
		"client":   schemaString("Harness that ran: omp, opencode or codex"),
		"agent":    schemaString("Profile or harness agent name used"),
		"model":    schemaString("Model the subagent ran"),
		"thinking": schemaString("Reasoning effort the subagent ran"),
		"task":     schemaString("Task description that was delegated"),
		"status":   schemaString("How the run ended"),
		"output":   schemaString("The subagent's final message"),
		"events":   schemaInt("Number of progress events streamed while it ran"),
		"elapsed":  schemaString("Wall-clock duration of the run"),
	},
	"client", "task", "status", "output", "events", "elapsed",
)

var permissionRuleOutputSchema = schemaObject(
	"Permission rule persisted by an always decision",
	map[string]any{
		"effect":  schemaString("allow or deny"),
		"tool":    schemaString("Tool the rule applies to"),
		"pattern": schemaString("Command pattern the rule matches"),
		"reason":  schemaString("Why the rule exists"),
	},
	"effect", "tool",
)

var permissionGrantOutputSchema = schemaObject(
	"Result of a permission_grant call",
	map[string]any{
		"request_id": schemaString("Request that was decided"),
		"granted":    schemaString("once, always or denied"),
		"rule":       permissionRuleOutputSchema,
		"message":    schemaString("How to proceed"),
	},
	"request_id", "granted",
)

// toolOutputSchemas is the declared output contract per built-in tool. A tool
// without an entry here fails fast when it is registered, so the surface can
// never advertise a tool whose structured output is undocumented.
var toolOutputSchemas = map[string]map[string]any{
	"list_devices": schemaObject(
		"Connected devices for the calling account",
		map[string]any{
			"devices": schemaArray(deviceOutputSchema, "Connected devices"),
			"count":   schemaInt("Number of devices returned"),
		},
		"devices", "count",
	),
	"list_workspaces": schemaObject(
		"Workspaces exposed by one device",
		map[string]any{
			"workspaces": schemaArray(workspaceOutputSchema, "Workspaces the device exposes"),
			"count":      schemaInt("Number of workspaces returned"),
		},
		"workspaces", "count",
	),
	"list": schemaObject(
		"Directory listing",
		map[string]any{
			"path":    schemaString("Workspace-relative directory that was listed"),
			"entries": schemaArray(dirEntryOutputSchema, "Directory entries"),
			"count":   schemaInt("Number of entries returned"),
		},
		"path", "entries", "count",
	),
	"read": schemaObject(
		"File contents",
		map[string]any{
			"path":       schemaString("Workspace-relative path that was read"),
			"content":    schemaString("File text"),
			"truncated":  schemaBool("True when the file exceeded the byte cap"),
			"bytes_read": schemaInt("Bytes returned in content"),
		},
		"path", "content", "truncated", "bytes_read",
	),
	"write":          patchOutputSchema,
	"edit":           patchOutputSchema,
	"apply_patch":    patchOutputSchema,
	"rollback_patch": rollbackOutputSchema,
	"bash":           bashOutputSchema,
	"agents_list": schemaObject(
		"Subagent profiles configured on the device",
		map[string]any{
			"agents": schemaArray(agentCatalogEntryOutputSchema, "Configured subagent profiles"),
		},
		"agents",
	),
	"agent":            agentOutputSchema,
	"permission_grant": permissionGrantOutputSchema,
}

// toolOutputPayload converts a payload received from a local client into the
// object shape the tool's schema declares. Tools whose client payload is a
// bare array are wrapped in their envelope, because structuredContent must be
// a JSON object.
//
// dirPath is the directory a list call asked for; it is echoed in the
// envelope so the result is self-describing.
func toolOutputPayload(tool, dirPath string, payload any) (any, error) {
	switch tool {
	case "list":
		entries, err := arrayPayload(tool, payload)
		if err != nil {
			return nil, err
		}
		if dirPath == "" {
			dirPath = "."
		}
		return map[string]any{"path": dirPath, "entries": entries, "count": len(entries)}, nil
	case "rollback_patch":
		// The client leaves restored_files as a JSON null when a checkpoint
		// restores nothing; the schema types it as an array.
		if object, ok := payload.(map[string]any); ok {
			for _, field := range []string{"restored_files"} {
				if value, present := object[field]; present && value == nil {
					object[field] = []any{}
				}
			}
		}
	}
	return payload, nil
}

// arrayPayload asserts that a client payload is a JSON array. A client that
// answers a list call with anything else would produce structured content that
// cannot conform to the declared schema, so it is reported as an error instead.
func arrayPayload(tool string, payload any) ([]any, error) {
	items, ok := payload.([]any)
	if !ok {
		return nil, errMalformedPayload(tool)
	}
	return items, nil
}
