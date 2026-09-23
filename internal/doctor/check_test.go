package doctor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestProductionChecks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write([]byte("ok\n"))
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              serverURL(r),
				"authorization_servers": []string{"https://auth.example.com"},
				"scopes_supported":      []string{"codebridge.read"},
			})
		case "/mcp":
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="https://codebridge.example/.well-known/oauth-protected-resource"`)
			http.Error(w, "no bearer token", http.StatusUnauthorized)
		case "/admin/devices":
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	results, ok := Run(context.Background(), Options{
		PublicURL:        server.URL,
		ExpectOAuth:      true,
		ExpectAdminBlock: true,
	})
	if !ok {
		t.Fatalf("doctor failed: %#v", results)
	}
}

func TestAdminDeviceCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write([]byte("ok\n"))
		case "/mcp":
			http.Error(w, "development endpoint", http.StatusUnauthorized)
		case "/admin/devices":
			if r.Header.Get("Authorization") != "Bearer secret" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "mac-1", "name": "Mac", "online": true},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	results, ok := Run(context.Background(), Options{
		PublicURL:        server.URL,
		ExpectOAuth:      false,
		ExpectAdminBlock: false,
		AdminURL:         server.URL,
		AdminToken:       "secret",
		DeviceID:         "mac-1",
	})
	if !ok {
		t.Fatalf("doctor failed: %#v", results)
	}
}

func TestNormalizeBaseURLRejectsPath(t *testing.T) {
	if _, err := normalizeBaseURL("https://example.com/mcp"); err == nil {
		t.Fatal("URL with path was accepted")
	}
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}

func TestAuthenticatedMCPSmoke(t *testing.T) {
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "doctor-test", Version: "test"}, nil)

	mcp.AddTool(mcpServer, &mcp.Tool{Name: "list_devices"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: `[{"id":"mac-1"}]`}},
		}, nil, nil
	})
	type deviceInput struct {
		DeviceID string `json:"device_id"`
	}
	mcp.AddTool(mcpServer, &mcp.Tool{Name: "list_workspaces"}, func(_ context.Context, _ *mcp.CallToolRequest, in deviceInput) (*mcp.CallToolResult, any, error) {
		if in.DeviceID != "mac-1" {
			t.Fatalf("unexpected device id %q", in.DeviceID)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: `[{"name":"demo"}]`}},
		}, nil, nil
	})
	type projectInput struct {
		DeviceID  string `json:"device_id"`
		Workspace string `json:"workspace"`
	}
	mcp.AddTool(mcpServer, &mcp.Tool{Name: "project_info"}, func(_ context.Context, _ *mcp.CallToolRequest, in projectInput) (*mcp.CallToolResult, any, error) {
		if in.DeviceID != "mac-1" || in.Workspace != "demo" {
			t.Fatalf("unexpected project_info input: %#v", in)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: `{"markers":["go.mod"],"has_git":true}`}},
		}, nil, nil
	})

	streamable := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return mcpServer
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resource":              serverURL(r),
			"authorization_servers": []string{"https://auth.example.com"},
			"scopes_supported":      []string{"codebridge.read"},
		})
	})
	mux.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer real-token" {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="https://codebridge.example/.well-known/oauth-protected-resource"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		streamable.ServeHTTP(w, r)
	}))
	mux.HandleFunc("/admin/devices", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	results, ok := Run(context.Background(), Options{
		PublicURL:        server.URL,
		AccessToken:      "real-token",
		DeviceID:         "mac-1",
		Workspace:        "demo",
		ExpectOAuth:      true,
		ExpectAdminBlock: true,
	})
	if !ok {
		t.Fatalf("authenticated doctor failed: %#v", results)
	}

	want := map[string]bool{
		"mcp_authenticated": false,
		"mcp_device":        false,
		"mcp_project_info":  false,
	}
	for _, result := range results {
		if _, exists := want[result.Name]; exists && result.OK {
			want[result.Name] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Fatalf("missing successful %s result: %#v", name, results)
		}
	}
}

func TestAuthenticatedMCPSmokeRejectsBadToken(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resource":              serverURL(r),
			"authorization_servers": []string{"https://auth.example.com"},
		})
	})
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="https://codebridge.example/.well-known/oauth-protected-resource"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	mux.HandleFunc("/admin/devices", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	results, ok := Run(context.Background(), Options{
		PublicURL:        server.URL,
		AccessToken:      "bad-token",
		ExpectOAuth:      true,
		ExpectAdminBlock: true,
	})
	if ok {
		t.Fatalf("bad access token unexpectedly passed: %#v", results)
	}
	foundFailure := false
	for _, result := range results {
		if result.Name == "mcp_authenticated" && !result.OK {
			foundFailure = true
			break
		}
	}
	if !foundFailure {
		t.Fatalf("missing authenticated MCP failure: %#v", results)
	}
}
