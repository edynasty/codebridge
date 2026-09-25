package manager

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/edynasty/codebridge/internal/authstore"
	"github.com/edynasty/codebridge/internal/protocol"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeAgent answers every forwarded request with a canned payload, so the
// manager-side tool surface can be exercised without a real local client.
type fakeAgent struct {
	conn     *AgentConn
	payloads map[string]string
}

func (f *fakeAgent) WriteJSON(v any) error {
	envelope, ok := v.(protocol.Envelope)
	if !ok {
		return nil
	}
	var req protocol.AgentRequest
	if err := json.Unmarshal(envelope.Payload, &req); err != nil {
		return err
	}
	payload, ok := f.payloads[req.Tool]
	if !ok {
		f.conn.Deliver(envelope.RequestID, protocol.AgentResponse{OK: false, Error: "no canned payload for " + req.Tool})
		return nil
	}
	f.conn.Deliver(envelope.RequestID, protocol.AgentResponse{OK: true, Data: json.RawMessage(payload)})
	return nil
}

func (f *fakeAgent) Close() error { return nil }

// cannedPayloads are the exact shapes the current local client returns, taken
// from its result types. A tool whose payload stops matching its declared
// output schema fails the test below.
var cannedPayloads = map[string]string{
	"list": `[{"name":"main.go","path":"main.go","type":"file","size":12},
	          {"name":"cmd","path":"cmd","type":"dir"}]`,
	"read": `{"path":"main.go","content":"package main\n","truncated":false,"bytes_read":13}`,
	"write": `{"applied":true,"files":[{"path":"new.txt","action":"create"}],
	           "checkpoint_id":"cbk_1","message":"Applied 1 file change(s)."}`,
	"edit": `{"applied":false,"preview":true,"diff":"@@ -1 +1 @@\n-a\n+b\n",
	          "files":[{"path":"main.go","action":"replace"}],"message":"Preview only."}`,
	"apply_patch": `{"applied":true,"files":[{"path":"main.go","action":"replace"}],
	                 "checkpoint_id":"cbk_2","message":"Applied."}`,
	"rollback_patch": `{"rolled_back":true,"checkpoint_id":"cbk_2","restored_files":["main.go"]}`,
	"bash":           `{"command":"echo hi","output":"hi\n","truncated":false}`,
	"agents_list":    `{"agents":[{"name":"plan","client":"omp","model":"local/kimi-k3:max","tab":"omp"}]}`,
	"agent": `{"client":"omp","agent":"plan","model":"local/kimi-k3:max","task":"do it",
	           "status":"completed","output":"done","events":7,"elapsed":"12.3s"}`,
	"permission_grant": `{"request_id":"req-1","granted":"always",
	                       "rule":{"effect":"allow","tool":"bash","pattern":"rg"},
	                       "message":"rule persisted; retry the blocked tool call now"}`,
}

// TestToolOutputsConformToDeclaredSchemas drives every forwarded tool through a
// real MCP session and asserts the server advertised an output schema and
// returned structuredContent validating against it. The SDK validates output
// against the declared schema, so a schema that drifts from the payload turns
// the call into an error and fails here.
func TestToolOutputsConformToDeclaredSchemas(t *testing.T) {
	registry := NewRegistry(4)
	fake := &fakeAgent{payloads: cannedPayloads}
	fake.conn = registry.Put(Device{
		ID:          "dev-1",
		AccountID:   authstore.DefaultAccount,
		Online:      true,
		Workspaces:  []protocol.Workspace{{Name: "ws", Writable: true}},
		ConnectedAt: time.Now(),
	}, fake)

	server := mcp.NewServer(&mcp.Implementation{Name: "CodeBridge", Version: "test"}, nil)
	(&ToolService{Registry: registry}).Register(server)
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "output-schema-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	advertised := map[string]*mcp.Tool{}
	for _, tool := range listed.Tools {
		advertised[tool.Name] = tool
	}
	for _, name := range builtinToolNames {
		tool, ok := advertised[name]
		if !ok {
			t.Fatalf("%s was not advertised", name)
		}
		schema, ok := tool.OutputSchema.(map[string]any)
		if !ok {
			t.Fatalf("%s has no output schema: %#v", name, tool.OutputSchema)
		}
		if schema["type"] != "object" {
			t.Fatalf("%s output schema is not an object: %#v", name, schema["type"])
		}
	}

	// Args that satisfy each tool's input schema; forwarded tools reuse the
	// canned payloads above.
	calls := map[string]map[string]any{
		"list":             {"path": "."},
		"read":             {"path": "main.go"},
		"write":            {"path": "new.txt", "content": "x", "confirm": false},
		"edit":             {"path": "main.go", "old_text": "a", "new_text": "b"},
		"apply_patch":      {"edits": []any{}},
		"rollback_patch":   {"checkpoint_id": "cbk_2"},
		"bash":             {"command": "echo hi"},
		"agents_list":      {},
		"agent":            {"task": "do it", "agent": "plan"},
		"permission_grant": {"request_id": "req-1", "decision": "always"},
	}
	for name, args := range calls {
		args["device_id"] = "dev-1"
		args["workspace"] = "ws"
		if name == "agents_list" {
			delete(args, "workspace")
		}
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if result.IsError {
			t.Fatalf("%s returned a tool error: %s", name, toolErrorText(result))
		}
		if result.StructuredContent == nil {
			t.Fatalf("%s returned no structuredContent", name)
		}
		// The spec asks for the serialized JSON in a text block as well.
		if len(result.Content) == 0 {
			t.Fatalf("%s returned no text content", name)
		}
	}

	// Device-scoped tools are served by the manager itself.
	for _, name := range []string{"list_devices", "list_workspaces"} {
		args := map[string]any{}
		if name == "list_workspaces" {
			args["device_id"] = "dev-1"
		}
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if result.IsError {
			t.Fatalf("%s returned a tool error: %s", name, toolErrorText(result))
		}
		structured, ok := result.StructuredContent.(map[string]any)
		if !ok {
			t.Fatalf("%s structuredContent = %#v", name, result.StructuredContent)
		}
		want := map[string]string{"list_devices": "devices", "list_workspaces": "workspaces"}[name]
		if _, ok := structured[want]; !ok {
			t.Fatalf("%s structuredContent missing %q: %#v", name, want, structured)
		}
	}
}

// toolErrorText renders a tool error result for test failures.
func toolErrorText(result *mcp.CallToolResult) string {
	var b strings.Builder
	for _, block := range result.Content {
		if text, ok := block.(*mcp.TextContent); ok {
			b.WriteString(text.Text)
		}
	}
	return b.String()
}

// TestListItemPayloadIsEnveloped pins the envelope for list, whose client
// payload is a bare array.
func TestListItemPayloadIsEnveloped(t *testing.T) {
	payload := []any{
		map[string]any{"name": "main.go", "path": "main.go", "type": "file", "size": float64(12)},
	}
	got, err := toolOutputPayload("list", "cmd", payload)
	if err != nil {
		t.Fatal(err)
	}
	object, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("list payload was not enveloped: %#v", got)
	}
	if object["path"] != "cmd" || object["count"] != 1 {
		t.Fatalf("envelope = %#v", object)
	}
	if entries, ok := object["entries"].([]any); !ok || len(entries) != 1 {
		t.Fatalf("envelope entries = %#v", object["entries"])
	}
}

// TestListItemPayloadRejectsNonArray guards the declared schema: a client that
// answers a list call with a non-array would otherwise emit structured content
// that cannot conform.
func TestListItemPayloadRejectsNonArray(t *testing.T) {
	if _, err := toolOutputPayload("list", ".", map[string]any{"entries": []any{}}); err == nil {
		t.Fatal("non-array list payload was accepted")
	}
}

// TestRollbackNullArraysBecomeEmpty pins the null coercion for the one field
// the client can emit as JSON null while the schema types it as an array.
func TestRollbackNullArraysBecomeEmpty(t *testing.T) {
	got, err := toolOutputPayload("rollback_patch", "", map[string]any{
		"rolled_back": true, "checkpoint_id": "cbk_1", "restored_files": nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	object := got.(map[string]any)
	if files, ok := object["restored_files"].([]any); !ok || len(files) != 0 {
		t.Fatalf("restored_files = %#v", object["restored_files"])
	}
}

// TestToolOutputSchemaIsEnforced proves the declared schemas are real: a client
// payload that does not match its tool's schema must fail the call instead of
// being forwarded as undocumented structured content.
func TestToolOutputSchemaIsEnforced(t *testing.T) {
	registry := NewRegistry(2)
	fake := &fakeAgent{payloads: map[string]string{
		// read requires path, content, truncated and bytes_read.
		"read": `{"path":"main.go"}`,
	}}
	fake.conn = registry.Put(Device{
		ID:          "dev-1",
		AccountID:   authstore.DefaultAccount,
		Workspaces:  []protocol.Workspace{{Name: "ws", Writable: true}},
		ConnectedAt: time.Now(),
	}, fake)

	server := mcp.NewServer(&mcp.Implementation{Name: "CodeBridge", Version: "test"}, nil)
	(&ToolService{Registry: registry}).Register(server)
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "output-schema-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "read",
		Arguments: map[string]any{"device_id": "dev-1", "workspace": "ws", "path": "main.go"},
	})
	if err == nil && (result == nil || !result.IsError) {
		t.Fatalf("a payload missing required fields was accepted: %#v", result)
	}
}
