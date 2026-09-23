package manager

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/edynasty/codebridge/internal/protocol"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ToolService struct {
	Registry *Registry
}

type DeviceInput struct {
	DeviceID string `json:"device_id" jsonschema:"ID of the connected device"`
}

type WorkspaceInput struct {
	DeviceID  string `json:"device_id" jsonschema:"ID of the connected device"`
	Workspace string `json:"workspace" jsonschema:"Workspace name exposed by the local agent"`
}

type PathInput struct {
	DeviceID  string `json:"device_id" jsonschema:"ID of the connected device"`
	Workspace string `json:"workspace" jsonschema:"Workspace name exposed by the local agent"`
	Path      string `json:"path,omitempty" jsonschema:"Path relative to the workspace root; absolute paths are rejected"`
}

type ReadFileInput struct {
	DeviceID  string `json:"device_id" jsonschema:"ID of the connected device"`
	Workspace string `json:"workspace" jsonschema:"Workspace name exposed by the local agent"`
	Path      string `json:"path" jsonschema:"File path relative to the workspace root"`
	MaxBytes  int    `json:"max_bytes,omitempty" jsonschema:"Maximum bytes to read; capped by the local agent"`
}

type FindFilesInput struct {
	DeviceID  string `json:"device_id" jsonschema:"ID of the connected device"`
	Workspace string `json:"workspace" jsonschema:"Workspace name exposed by the local agent"`
	Pattern   string `json:"pattern" jsonschema:"Filename glob or case-insensitive path substring"`
}

type SearchCodeInput struct {
	DeviceID  string `json:"device_id" jsonschema:"ID of the connected device"`
	Workspace string `json:"workspace" jsonschema:"Workspace name exposed by the local agent"`
	Query     string `json:"query" jsonschema:"Literal text to search for in source files"`
	Path      string `json:"path,omitempty" jsonschema:"Optional subdirectory relative to the workspace root"`
}

func readOnlyTool(name, description string) *mcp.Tool {
	return &mcp.Tool{
		Name:        name,
		Description: description,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}
}

func textResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, v, nil
}

func (t *ToolService) Register(server *mcp.Server) {
	mcp.AddTool(server, readOnlyTool("list_devices", "List local CodeBridge devices currently connected to this manager."), func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, any, error) {
		return textResult(t.Registry.List())
	})

	mcp.AddTool(server, readOnlyTool("list_workspaces", "List source-code workspaces exposed by one connected device."), func(ctx context.Context, req *mcp.CallToolRequest, in DeviceInput) (*mcp.CallToolResult, any, error) {
		v, err := t.Registry.Workspaces(in.DeviceID)
		if err != nil {
			return nil, nil, err
		}
		return textResult(v)
	})

	mcp.AddTool(server, readOnlyTool("list_directory", "List files and directories inside an exposed local workspace. Paths are workspace-relative."), func(ctx context.Context, req *mcp.CallToolRequest, in PathInput) (*mcp.CallToolResult, any, error) {
		return t.forward(ctx, in.DeviceID, in.Workspace, "list_directory", map[string]any{"path": in.Path})
	})

	mcp.AddTool(server, readOnlyTool("read_file", "Read a text/source file from an exposed local workspace. The local agent enforces path boundaries and size limits."), func(ctx context.Context, req *mcp.CallToolRequest, in ReadFileInput) (*mcp.CallToolResult, any, error) {
		return t.forward(ctx, in.DeviceID, in.Workspace, "read_file", map[string]any{"path": in.Path, "max_bytes": in.MaxBytes})
	})

	mcp.AddTool(server, readOnlyTool("find_files", "Find files by filename glob or path substring inside an exposed local workspace."), func(ctx context.Context, req *mcp.CallToolRequest, in FindFilesInput) (*mcp.CallToolResult, any, error) {
		return t.forward(ctx, in.DeviceID, in.Workspace, "find_files", map[string]any{"pattern": in.Pattern})
	})

	mcp.AddTool(server, readOnlyTool("search_code", "Search literal text in local source code. Uses ripgrep when installed and never executes user-provided shell commands."), func(ctx context.Context, req *mcp.CallToolRequest, in SearchCodeInput) (*mcp.CallToolResult, any, error) {
		return t.forward(ctx, in.DeviceID, in.Workspace, "search_code", map[string]any{"query": in.Query, "path": in.Path})
	})

	mcp.AddTool(server, readOnlyTool("git_status", "Return git status for an exposed local workspace."), func(ctx context.Context, req *mcp.CallToolRequest, in WorkspaceInput) (*mcp.CallToolResult, any, error) {
		return t.forward(ctx, in.DeviceID, in.Workspace, "git_status", nil)
	})

	mcp.AddTool(server, readOnlyTool("git_diff", "Return the unstaged git diff for an exposed local workspace, optionally limited to one workspace-relative path."), func(ctx context.Context, req *mcp.CallToolRequest, in PathInput) (*mcp.CallToolResult, any, error) {
		return t.forward(ctx, in.DeviceID, in.Workspace, "git_diff", map[string]any{"path": in.Path})
	})

	mcp.AddTool(server, readOnlyTool("project_info", "Detect common project/build markers such as pom.xml, go.mod, package.json and Dockerfile."), func(ctx context.Context, req *mcp.CallToolRequest, in WorkspaceInput) (*mcp.CallToolResult, any, error) {
		return t.forward(ctx, in.DeviceID, in.Workspace, "project_info", nil)
	})
}

func (t *ToolService) forward(ctx context.Context, deviceID, workspace, tool string, args map[string]any) (*mcp.CallToolResult, any, error) {
	if deviceID == "" || workspace == "" {
		return nil, nil, fmt.Errorf("device_id and workspace are required")
	}
	raw, err := t.Registry.Call(ctx, deviceID, protocolRequest(tool, workspace, args))
	if err != nil {
		return nil, nil, err
	}
	var out any
	if len(raw) == 0 {
		out = map[string]any{"ok": true}
	} else if err := json.Unmarshal(raw, &out); err != nil {
		out = string(raw)
	}
	return textResult(out)
}

func protocolRequest(tool, workspace string, args map[string]any) protocol.AgentRequest {
	if args == nil {
		args = map[string]any{}
	}
	return protocol.AgentRequest{Tool: tool, Workspace: workspace, Args: args}
}

func boolPtr(v bool) *bool { return &v }
