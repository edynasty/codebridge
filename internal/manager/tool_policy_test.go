package manager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/edynasty/codebridge/internal/protocol"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The patch tools must be advertised as mutating so MCP clients can warn users
// before calling them, and they must follow the device's tool policy.
func TestPatchToolsAdvertisedAndPolicyGated(t *testing.T) {
	registry := NewRegistry(4)
	serverImpl := mcp.NewServer(&mcp.Implementation{Name: "CodeBridge", Version: "test"}, nil)
	(&ToolService{Registry: registry}).Register(serverImpl)

	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return serverImpl },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "tool-policy-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	advertised := map[string]*mcp.Tool{}
	for _, tool := range tools.Tools {
		advertised[tool.Name] = tool
	}
	for _, name := range []string{"apply_patch", "rollback_patch"} {
		tool, ok := advertised[name]
		if !ok {
			t.Fatalf("%s was not advertised", name)
		}
		if tool.Annotations == nil || tool.Annotations.ReadOnlyHint {
			t.Fatalf("%s is not advertised as mutating: %#v", name, tool.Annotations)
		}
	}

	// A device that enabled the patch tools exposes them; a tool the device did
	// not enable stays hidden.
	registry.Put(Device{ID: "dev-write", ToolPolicy: &protocol.ToolPolicy{
		EnabledTools: []string{"read", "write", "apply_patch", "rollback_patch"},
	}}, nil)
	svc := &ToolService{Registry: registry}
	for _, name := range []string{"apply_patch", "rollback_patch"} {
		if svc.toolDisabledForAccount("", name) {
			t.Fatalf("%s hidden although the device enabled it", name)
		}
	}
	if !svc.toolDisabledForAccount("", "bash") {
		t.Fatal("bash exposed although the device did not enable it")
	}
}
