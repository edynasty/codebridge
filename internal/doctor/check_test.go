package doctor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
