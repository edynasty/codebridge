package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/edynasty/codebridge/internal/agentops"
	"github.com/edynasty/codebridge/internal/authstore"
	"github.com/edynasty/codebridge/internal/manager"
	"github.com/edynasty/codebridge/internal/oauthresource"
	"github.com/edynasty/codebridge/internal/protocol"
	"github.com/golang-jwt/jwt/v5"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestOAuthAccountScopingCrossTenant proves MGR-001: an authenticated subject
// cannot discover or call another account's device even when it knows the
// device ID, while a mapped subject of the owning account can.
func TestOAuthAccountScopingCrossTenant(t *testing.T) {
	workspace := t.TempDir()
	const wantContent = "scoped account content\n"
	if err := os.WriteFile(filepath.Join(workspace, "scoped.txt"), []byte(wantContent), 0o600); err != nil {
		t.Fatal(err)
	}

	signingKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "scoping-key"
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jwks" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []any{integrationRSAJWK(kid, &signingKey.PublicKey)},
		})
	}))
	defer jwksServer.Close()

	const resource = "https://codebridge.scoping.test"
	verifier, err := oauthresource.New(oauthresource.Config{
		Issuer:          jwksServer.URL,
		Audience:        resource,
		JWKSURL:         jwksServer.URL + "/jwks",
		AllowedSubjects: []string{"owner-user", "intruder-user", "mapped-user"},
	})
	if err != nil {
		t.Fatal(err)
	}

	authStore, err := authstore.Open(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	enrollmentCode, _, err := authStore.CreateEnrollment(time.Minute, "owner-user")
	if err != nil {
		t.Fatal(err)
	}

	registry := manager.NewRegistry(4)
	toolService := &manager.ToolService{
		Registry: registry,
		Accounts: &manager.AccountResolver{Map: map[string]string{
			"mapped-user": "owner-user",
		}},
		OAuthScopes: []string{"codebridge.read"},
	}
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "CodeBridge Scoping", Version: "test"}, nil)
	toolService.Register(mcpServer)
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return mcpServer
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	protectedMCP := mcpauth.RequireBearerToken(verifier.Verify, &mcpauth.RequireBearerTokenOptions{
		ResourceMetadataURL: resource + "/.well-known/oauth-protected-resource",
		Scopes:              []string{"codebridge.read"},
		ClockSkew:           30 * time.Second,
	})(mcpHandler)

	mux := http.NewServeMux()
	mux.Handle("/mcp", protectedMCP)
	mux.Handle("/agent", &manager.AgentHandler{Registry: registry, Auth: authStore})
	server := httptest.NewServer(mux)
	defer server.Close()

	agentCtx, agentCancel := context.WithCancel(context.Background())
	defer agentCancel()
	agentDone := make(chan error, 1)
	go func() {
		agentDone <- runSession(agentCtx, "ws"+strings.TrimPrefix(server.URL, "http")+"/agent", false, protocol.RegisterRequest{
			EnrollmentCode: enrollmentCode,
			DeviceID:       "scoped-device",
			DeviceName:     "Scoping Test Device",
			Version:        "test",
			Workspaces:     []protocol.Workspace{{Name: "demo"}},
		}, &agentops.Service{Roots: map[string]string{"demo": workspace}}, func(string) error { return nil })
	}()
	waitForDeviceState(t, registry, "scoped-device", true)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	mintToken := func(subject string) string {
		t.Helper()
		now := time.Now()
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss":   jwksServer.URL,
			"aud":   resource,
			"sub":   subject,
			"exp":   now.Add(5 * time.Minute).Unix(),
			"nbf":   now.Add(-time.Minute).Unix(),
			"iat":   now.Unix(),
			"scope": "codebridge.read",
		})
		token.Header["kid"] = kid
		signed, err := token.SignedString(signingKey)
		if err != nil {
			t.Fatal(err)
		}
		return signed
	}
	connect := func(subject string) *mcp.ClientSession {
		t.Helper()
		httpClient := &http.Client{Transport: bearerRoundTripper{
			token: mintToken(subject),
			base:  http.DefaultTransport,
		}}
		session, err := mcp.NewClient(&mcp.Implementation{
			Name:    "scoping-client",
			Version: "test",
		}, nil).Connect(ctx, &mcp.StreamableClientTransport{
			Endpoint:   server.URL + "/mcp",
			HTTPClient: httpClient,
		}, nil)
		if err != nil {
			t.Fatalf("connect MCP client for %s: %v", subject, err)
		}
		t.Cleanup(func() { _ = session.Close() })
		return session
	}

	// The intruder knows the exact device ID but is a different account.
	intruder := connect("intruder-user")
	result, err := intruder.CallTool(ctx, &mcp.CallToolParams{Name: "list_devices"})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("intruder list_devices errored: %#v", result.Content)
	}
	if !deviceListEmpty(t, result) {
		t.Fatal("intruder saw another account's device in list_devices")
	}
	if deviceVisible(t, ctx, intruder, "list_workspaces", map[string]any{"device_id": "scoped-device"}) {
		t.Fatal("intruder discovered workspaces of another account's device")
	}
	if deviceVisible(t, ctx, intruder, "read_file", map[string]any{
		"device_id": "scoped-device",
		"workspace": "demo",
		"path":      "scoped.txt",
	}) {
		t.Fatal("intruder read a file through another account's device")
	}

	// A subject mapped to the owning account retains full access.
	owner := connect("mapped-user")
	result, err = owner.CallTool(ctx, &mcp.CallToolParams{Name: "list_devices"})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || deviceListEmpty(t, result) {
		t.Fatalf("mapped owner could not list devices: %#v", result.Content)
	}
	result, err = owner.CallTool(ctx, &mcp.CallToolParams{
		Name: "read_file",
		Arguments: map[string]any{
			"device_id": "scoped-device",
			"workspace": "demo",
			"path":      "scoped.txt",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("mapped owner read_file failed: %#v", result.Content)
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected content type %T", result.Content[0])
	}
	var read agentops.ReadFileResult
	if err := json.Unmarshal([]byte(textContent.Text), &read); err != nil {
		t.Fatalf("decode read_file result: %v: %s", err, textContent.Text)
	}
	if read.Content != wantContent {
		t.Fatalf("unexpected read_file content: %q", read.Content)
	}

	agentCancel()
	select {
	case <-agentDone:
	case <-time.After(5 * time.Second):
		t.Fatal("agent did not stop after cancellation")
	}
}

func deviceListEmpty(t *testing.T, result *mcp.CallToolResult) bool {
	t.Helper()
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected content type %T", result.Content[0])
	}
	var devices []manager.Device
	if err := json.Unmarshal([]byte(textContent.Text), &devices); err != nil {
		t.Fatalf("decode device list: %v: %s", err, textContent.Text)
	}
	return len(devices) == 0
}

func deviceVisible(t *testing.T, ctx context.Context, session *mcp.ClientSession, tool string, args map[string]any) bool {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s call failed at protocol level: %v", tool, err)
	}
	return !result.IsError
}
