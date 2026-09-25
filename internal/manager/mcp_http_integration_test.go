package manager

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestStreamableHTTPWithOfficialMCPClient(t *testing.T) {
	registry := NewRegistry(4)
	serverImpl := mcp.NewServer(&mcp.Implementation{
		Name:    "CodeBridge",
		Version: "test",
	}, nil)
	(&ToolService{Registry: registry}).Register(serverImpl)

	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return serverImpl },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "integration-client",
		Version: "test",
	}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: httpServer.URL,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range tools.Tools {
		if tool.Name == "list_devices" {
			found = true
			if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
				t.Fatalf("list_devices is not advertised read-only: %#v", tool.Annotations)
			}
		}
	}
	if !found {
		t.Fatal("list_devices tool was not advertised")
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_devices"})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || len(result.Content) == 0 {
		t.Fatalf("unexpected tool result: %#v", result)
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected content type %T", result.Content[0])
	}
	var listed struct {
		Devices []Device `json:"devices"`
		Count   int      `json:"count"`
	}
	if err := json.Unmarshal([]byte(textContent.Text), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Devices) != 0 || listed.Count != 0 {
		t.Fatalf("expected empty device list, got %#v", listed)
	}
	if result.StructuredContent == nil {
		t.Fatal("list_devices returned no structuredContent")
	}
}
