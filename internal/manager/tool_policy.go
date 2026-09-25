package manager

import (
	"context"
	"fmt"
	"net/http"

	"github.com/edynasty/codebridge/internal/protocol"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// builtinToolNames is the ordered registry of built-in MCP tools. Custom tool
// definitions may only wrap names on this list.
var builtinToolNames = []string{
	"list_devices", "list_workspaces", "list", "read", "edit", "write",
	"apply_patch", "rollback_patch", "bash",
	"agents_list", "agent", "permission_grant",
}

// piCoreTools is the pi-style default surface (badlogic/pi-mono's essential
// four — read, write, edit, bash — plus ls and the discovery pair).
// Clients that never configured a tool policy get this set.
var piCoreTools = map[string]bool{
	"list_devices": true, "list_workspaces": true, "list": true,
	"read": true, "edit": true, "write": true, "bash": true,
	"agents_list": true, "agent": true, "permission_grant": true,
}

func isBuiltinTool(name string) bool {
	for _, n := range builtinToolNames {
		if n == name {
			return true
		}
	}
	return false
}

func (t *ToolService) toolDisabledForAccount(account, tool string) bool {
	enabledSeen := false
	for _, d := range t.Registry.ListAccount(account) {
		policy := d.ToolPolicy
		if policy == nil {
			// No policy = default pi-style core set.
			if !piCoreTools[tool] {
				return true
			}
			continue
		}
		for _, name := range policy.DisabledTools {
			if name == tool {
				return true
			}
		}
		if len(policy.EnabledTools) == 0 {
			if !piCoreTools[tool] {
				return true
			}
			continue
		}
		enabledSeen = true
		all := false
		hit := false
		for _, name := range policy.EnabledTools {
			if name == "*" {
				all = true
			}
			if name == tool {
				hit = true
			}
		}
		if !all && !hit {
			return true
		}
	}
	_ = enabledSeen
	return false
}

func (t *ToolService) customToolsForAccount(account string) []protocol.CustomTool {
	var out []protocol.CustomTool
	seen := map[string]bool{}
	for _, d := range t.Registry.ListAccount(account) {
		if d.ToolPolicy == nil {
			continue
		}
		for _, custom := range d.ToolPolicy.CustomTools {
			if seen[custom.Name] || !isBuiltinTool(custom.Tool) || custom.Name == "" {
				continue
			}
			seen[custom.Name] = true
			out = append(out, custom)
		}
	}
	return out
}

// serverVersion is set by cmd/manager so per-request servers report the
// build version like the static one did.
var serverVersion = "dev"

// ServerVersion records the build version for per-request servers.
func ServerVersion(v string) { serverVersion = v }

// AccountFromRequest resolves the caller account from one MCP HTTP request.
// cmd/manager installs the concrete resolver (static token -> default
// account; OAuth -> subject-derived account); without one every request maps
// to the default account.
var AccountFromRequest = func(r *http.Request) string { return "" }

// ServerForRequest builds the MCP server for one HTTP request. The tool
// surface reflects the caller's account: a built-in tool is hidden only when
// every device of that account disabled it (intersection semantics, so one
// restrictive device cannot hide tools for the whole account), and custom
// tool wrappers from any of the account's devices are exposed (union).
func (t *ToolService) ServerForRequest(r *http.Request) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "CodeBridge", Version: serverVersion}, nil)
	t.registerWithPolicy(server, AccountFromRequest(r))
	return server
}

// registerWithPolicy registers every built-in tool not disabled for the
// account, plus each custom tool wrapper. Callers with no devices keep the
// full built-in set so tools/list works before the first device registers.
func (t *ToolService) registerWithPolicy(server *mcp.Server, account string) {
	disabled := map[string]bool{}
	for _, name := range builtinToolNames {
		if t.toolDisabledForAccount(account, name) {
			disabled[name] = true
		}
	}
	for _, name := range builtinToolNames {
		if disabled[name] {
			continue
		}
		t.RegisterOne(server, name)
	}
	for _, custom := range t.customToolsForAccount(account) {
		t.registerCustom(server, custom)
	}
}

// registerCustom exposes one operator-defined tool that forwards to a preset
// built-in tool with fixed arguments. Caller-supplied arguments are merged on
// top of the preset (preset wins is NOT the rule: caller args override preset
// keys except device_id/workspace, which stay pinned), so wrappers can expose
// a narrowed surface while still allowing optional parameters.
func (t *ToolService) registerCustom(server *mcp.Server, custom protocol.CustomTool) {
	desc := custom.Description
	if desc == "" {
		desc = fmt.Sprintf("Custom CodeBridge method wrapping %s.", custom.Tool)
	}
	// A wrapper reports the output of the built-in it forwards to, and
	// readOnlyTool would look up the schema under the wrapper's own name.
	tool := t.annotatedTool(custom.Name, desc, true, false, custom.Tool)
	mcp.AddTool(server, tool, func(ctx context.Context, req *mcp.CallToolRequest, in map[string]any) (*mcp.CallToolResult, any, error) {
		account := t.accountFor(req)
		args := map[string]any{}
		for k, v := range custom.Args {
			args[k] = v
		}
		for k, v := range in {
			if k == "device_id" || k == "workspace" {
				continue
			}
			args[k] = v
		}
		deviceID := stringArg(args, "device_id", "")
		workspace := stringArg(args, "workspace", "")
		return t.invokeArgs(req, custom.Name, deviceID, workspace, args, func() (*mcp.CallToolResult, any, error) {
			return t.forward(ctx, account, deviceID, workspace, custom.Tool, args)
		})
	})
}

func stringArg(m map[string]any, key, def string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return def
}
