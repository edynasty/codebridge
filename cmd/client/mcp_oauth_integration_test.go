package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/edynasty/codebridge/internal/agentops"
	"github.com/edynasty/codebridge/internal/auditlog"
	"github.com/edynasty/codebridge/internal/authstore"
	"github.com/edynasty/codebridge/internal/manager"
	"github.com/edynasty/codebridge/internal/oauthresource"
	"github.com/edynasty/codebridge/internal/protocol"
	"github.com/golang-jwt/jwt/v5"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestOAuthMCPToRealClientReadFileEndToEnd(t *testing.T) {
	workspace := t.TempDir()
	const wantContent = "oauth mcp to websocket client\n"
	if err := os.WriteFile(filepath.Join(workspace, "secret.txt"), []byte(wantContent), 0o600); err != nil {
		t.Fatal(err)
	}

	signingKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "integration-key"
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

	const resource = "https://codebridge.integration.test"
	verifier, err := oauthresource.New(oauthresource.Config{
		Issuer:          jwksServer.URL,
		Audience:        resource,
		JWKSURL:         jwksServer.URL + "/jwks",
		AllowedSubjects: []string{"integration-user"},
	})
	if err != nil {
		t.Fatal(err)
	}

	authStore, err := authstore.Open(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	enrollmentCode, _, err := authStore.CreateEnrollment(time.Minute, "integration-user")
	if err != nil {
		t.Fatal(err)
	}

	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	audit, err := auditlog.New(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	defer audit.Close()

	registry := manager.NewRegistry(4)
	toolService := &manager.ToolService{
		Registry:    registry,
		OAuthScopes: []string{"codebridge.read"},
		Audit:       audit,
	}
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "CodeBridge Integration", Version: "test"}, nil)
	toolService.Register(mcpServer)
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return mcpServer
	}, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
	})
	protectedMCP := mcpauth.RequireBearerToken(verifier.Verify, &mcpauth.RequireBearerTokenOptions{
		ResourceMetadataURL: resource + "/.well-known/oauth-protected-resource",
		Scopes:              []string{"codebridge.read"},
		ClockSkew:           30 * time.Second,
	})(mcpHandler)

	mux := http.NewServeMux()
	mux.Handle("/mcp", protectedMCP)
	mux.Handle("/agent", &manager.AgentHandler{
		Registry: registry,
		Auth:     authStore,
		Audit:    audit,
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	agentCtx, agentCancel := context.WithCancel(context.Background())
	defer agentCancel()
	agentDone := make(chan error, 1)
	issued := make(chan string, 1)
	go func() {
		agentDone <- runSession(agentCtx, "ws"+strings.TrimPrefix(server.URL, "http")+"/agent", protocol.RegisterRequest{
			EnrollmentCode: enrollmentCode,
			DeviceID:       "oauth-device",
			DeviceName:     "OAuth Integration Device",
			Version:        "test",
			Workspaces:     []protocol.Workspace{{Name: "demo"}},
		}, &agentops.Service{Roots: map[string]string{"demo": workspace}}, func(credential string) error {
			issued <- credential
			return nil
		})
	}()

	select {
	case credential := <-issued:
		if !authStore.VerifyDevice("oauth-device", credential) {
			t.Fatal("issued client credential did not verify")
		}
	case err := <-agentDone:
		t.Fatalf("agent stopped before enrollment completed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for client enrollment")
	}
	waitForDeviceState(t, registry, "oauth-device", true)

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":   jwksServer.URL,
		"aud":   resource,
		"sub":   "integration-user",
		"exp":   now.Add(5 * time.Minute).Unix(),
		"nbf":   now.Add(-time.Minute).Unix(),
		"iat":   now.Unix(),
		"scope": "codebridge.read",
	})
	token.Header["kid"] = kid
	signedToken, err := token.SignedString(signingKey)
	if err != nil {
		t.Fatal(err)
	}

	httpClient := &http.Client{Transport: bearerRoundTripper{
		token: signedToken,
		base:  http.DefaultTransport,
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := mcp.NewClient(&mcp.Implementation{
		Name:    "CodeBridge Integration Client",
		Version: "test",
	}, nil).Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   server.URL + "/mcp",
		HTTPClient: httpClient,
	}, nil)
	if err != nil {
		t.Fatalf("connect authenticated MCP client: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	foundReadFile := false
	for _, tool := range tools.Tools {
		if tool.Name == "read_file" {
			foundReadFile = true
			break
		}
	}
	if !foundReadFile {
		t.Fatal("read_file tool was not advertised")
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "read_file",
		Arguments: map[string]any{
			"device_id": "oauth-device",
			"workspace": "demo",
			"path":      "secret.txt",
		},
	})
	if err != nil {
		t.Fatalf("call read_file through OAuth MCP: %v", err)
	}
	if result.IsError {
		t.Fatalf("read_file returned MCP tool error: %#v", result.Content)
	}
	if len(result.Content) == 0 {
		t.Fatal("read_file returned no MCP content")
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected MCP content type %T", result.Content[0])
	}
	var read agentops.ReadFileResult
	if err := json.Unmarshal([]byte(textContent.Text), &read); err != nil {
		t.Fatalf("decode read_file MCP content: %v: %s", err, textContent.Text)
	}
	if read.Content != wantContent || read.Path != "secret.txt" {
		t.Fatalf("unexpected read_file result: %#v", read)
	}

	auditBytes, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	var sawToolAudit bool
	for _, line := range strings.Split(strings.TrimSpace(string(auditBytes)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var event auditlog.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode audit event: %v", err)
		}
		if event.Event == "mcp.tool" && event.Tool == "read_file" {
			sawToolAudit = true
			if event.ActorID != "integration-user" || event.DeviceID != "oauth-device" || event.Workspace != "demo" {
				t.Fatalf("unexpected MCP audit metadata: %#v", event)
			}
		}
	}
	if !sawToolAudit {
		t.Fatal("missing mcp.tool audit event")
	}

	agentCancel()
	select {
	case <-agentDone:
	case <-time.After(5 * time.Second):
		t.Fatal("agent did not stop after cancellation")
	}
	waitForDeviceState(t, registry, "oauth-device", false)
}

type bearerRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (b bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header.Set("Authorization", "Bearer "+b.token)
	base := b.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(cloned)
}

func integrationRSAJWK(kid string, pub *rsa.PublicKey) map[string]any {
	return map[string]any{
		"kty": "RSA",
		"kid": kid,
		"use": "sig",
		"alg": "RS256",
		"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}
}
