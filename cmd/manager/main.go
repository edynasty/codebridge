package main

import (
	"crypto/subtle"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/edynasty/codebridge/internal/auditlog"
	"github.com/edynasty/codebridge/internal/authstore"
	mgr "github.com/edynasty/codebridge/internal/manager"
	"github.com/edynasty/codebridge/internal/oauthresource"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

var version = "dev"

type oauthConfig struct {
	PublicURL       string
	Resource        string
	Issuer          string
	JWKSURL         string
	Scope           string
	AllowedSubjects []string
}

func main() {
	addr := flag.String("addr", env("CODEBRIDGE_ADDR", ":8080"), "listen address")
	stateFile := flag.String("state-file", env("CODEBRIDGE_STATE_FILE", "./data/auth.json"), "device auth state file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	audit, err := auditlog.New(strings.TrimSpace(os.Getenv("CODEBRIDGE_AUDIT_LOG")))
	if err != nil {
		log.Fatalf("open audit log: %v", err)
	}
	defer audit.Close()

	deviceAuth, err := authstore.Open(*stateFile)
	if err != nil {
		log.Fatalf("open auth state: %v", err)
	}
	oauthCfg, err := loadOAuthConfig()
	if err != nil {
		log.Fatalf("OAuth configuration: %v", err)
	}

	maxDeviceInflight := envInt("CODEBRIDGE_MAX_INFLIGHT_PER_DEVICE", 8, 1, 64)
	maxMCPInflight := envInt("CODEBRIDGE_MAX_MCP_INFLIGHT", 16, 1, 128)
	maxMCPRequestBytes := envInt("CODEBRIDGE_MAX_MCP_REQUEST_BYTES", 1024*1024, 64*1024, 8*1024*1024)

	registry := mgr.NewRegistry(maxDeviceInflight)
	toolService := &mgr.ToolService{Registry: registry, Audit: audit}
	if oauthCfg != nil {
		toolService.OAuthScopes = []string{oauthCfg.Scope}
	}
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "CodeBridge", Version: version}, nil)
	toolService.Register(mcpServer)

	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server { return mcpServer }, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
	})

	mux := http.NewServeMux()
	var protectedMCP http.Handler = mcpHandler
	if oauthCfg != nil {
		verifier, err := oauthresource.New(oauthresource.Config{
			Issuer:          oauthCfg.Issuer,
			Audience:        oauthCfg.Resource,
			JWKSURL:         oauthCfg.JWKSURL,
			AllowedSubjects: oauthCfg.AllowedSubjects,
		})
		if err != nil {
			log.Fatalf("OAuth verifier: %v", err)
		}
		metadataURL := oauthCfg.PublicURL + "/.well-known/oauth-protected-resource"
		metadata := &oauthex.ProtectedResourceMetadata{
			Resource:               oauthCfg.Resource,
			AuthorizationServers:   []string{oauthCfg.Issuer},
			ScopesSupported:        []string{oauthCfg.Scope},
			BearerMethodsSupported: []string{"header"},
			ResourceName:           "CodeBridge",
		}
		metaHandler := mcpauth.ProtectedResourceMetadataHandler(metadata)
		mux.Handle("/.well-known/oauth-protected-resource", metaHandler)
		// RFC 9728 path-form discovery for clients that derive metadata from /mcp.
		mux.Handle("/.well-known/oauth-protected-resource/mcp", metaHandler)
		protectedMCP = mcpauth.RequireBearerToken(verifier.Verify, &mcpauth.RequireBearerTokenOptions{
			ResourceMetadataURL: metadataURL,
			Scopes:              []string{oauthCfg.Scope},
			ClockSkew:           30 * time.Second,
		})(mcpHandler)
		log.Printf("MCP OAuth enabled: issuer=%s resource=%s scope=%s allowed_subjects=%d", oauthCfg.Issuer, oauthCfg.Resource, oauthCfg.Scope, len(oauthCfg.AllowedSubjects))
	} else {
		protectedMCP = optionalBearer(os.Getenv("CODEBRIDGE_MCP_TOKEN"), mcpHandler)
		if os.Getenv("CODEBRIDGE_MCP_TOKEN") == "" {
			log.Printf("WARNING: MCP authentication is disabled; do not expose /mcp to the public internet")
		} else {
			log.Printf("MCP static bearer enabled for development; OAuth is recommended for ChatGPT linking")
		}
	}

	mux.Handle("/mcp", withRequestID(limitMCP(maxMCPInflight, int64(maxMCPRequestBytes), protectedMCP)))
	mux.Handle("/agent", withRequestID(&mgr.AgentHandler{Registry: registry, Auth: deviceAuth, Audit: audit}))
	mux.Handle("/admin/", withRequestID(&mgr.AdminHandler{Auth: deviceAuth, Registry: registry, AdminToken: os.Getenv("CODEBRIDGE_ADMIN_TOKEN"), Audit: audit}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"CodeBridge","mcp":"/mcp","agent":"/agent","health":"/healthz"}`))
	})

	s := &http.Server{
		Addr:              *addr,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      45 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("CodeBridge manager listening on %s", *addr)
	log.Printf("MCP endpoint: /mcp; agent websocket: /agent")
	log.Printf("limits: mcp_inflight=%d device_inflight=%d mcp_request_bytes=%d", maxMCPInflight, maxDeviceInflight, maxMCPRequestBytes)
	if os.Getenv("CODEBRIDGE_ADMIN_TOKEN") == "" {
		log.Printf("admin API disabled: CODEBRIDGE_ADMIN_TOKEN is empty")
	} else {
		log.Printf("admin API enabled at /admin/")
	}
	if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func loadOAuthConfig() (*oauthConfig, error) {
	publicURL := strings.TrimRight(strings.TrimSpace(os.Getenv("CODEBRIDGE_PUBLIC_URL")), "/")
	issuer := strings.TrimSpace(os.Getenv("CODEBRIDGE_OAUTH_ISSUER"))
	jwksURL := strings.TrimSpace(os.Getenv("CODEBRIDGE_OAUTH_JWKS_URL"))
	resource := strings.TrimSpace(os.Getenv("CODEBRIDGE_OAUTH_RESOURCE"))
	scope := strings.TrimSpace(env("CODEBRIDGE_OAUTH_SCOPE", "codebridge.read"))
	allowedRaw := strings.TrimSpace(os.Getenv("CODEBRIDGE_OAUTH_ALLOWED_SUBJECTS"))

	enabled := publicURL != "" || issuer != "" || jwksURL != "" || resource != "" || allowedRaw != ""
	if !enabled {
		return nil, nil
	}
	if publicURL == "" || issuer == "" || jwksURL == "" {
		return nil, errors.New("CODEBRIDGE_PUBLIC_URL, CODEBRIDGE_OAUTH_ISSUER and CODEBRIDGE_OAUTH_JWKS_URL are all required when OAuth is enabled")
	}
	if resource == "" {
		resource = publicURL
	}
	if scope == "" || len(strings.Fields(scope)) != 1 {
		return nil, errors.New("CODEBRIDGE_OAUTH_SCOPE must contain exactly one OAuth scope value")
	}
	if err := validatePublicURL(publicURL); err != nil {
		return nil, err
	}
	if err := validateResourceURI(resource); err != nil {
		return nil, err
	}
	allowedSubjects, err := parseCSVValues(allowedRaw, 100, 256)
	if err != nil {
		return nil, fmt.Errorf("CODEBRIDGE_OAUTH_ALLOWED_SUBJECTS: %w", err)
	}
	return &oauthConfig{
		PublicURL:       publicURL,
		Resource:        resource,
		Issuer:          issuer,
		JWKSURL:         jwksURL,
		Scope:           scope,
		AllowedSubjects: allowedSubjects,
	}, nil
}

func parseCSVValues(raw string, maxItems, maxLen int) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		if len(value) > maxLen {
			return nil, fmt.Errorf("value exceeds %d bytes", maxLen)
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
		if len(out) > maxItems {
			return nil, fmt.Errorf("too many values; maximum is %d", maxItems)
		}
	}
	return out, nil
}

func validateResourceURI(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.Fragment != "" {
		return errors.New("CODEBRIDGE_OAUTH_RESOURCE must be an absolute HTTPS URI without a fragment")
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" {
		host := u.Hostname()
		if host == "localhost" {
			return nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil
		}
	}
	return errors.New("CODEBRIDGE_OAUTH_RESOURCE must use https (http is allowed only for loopback development)")
}

func validatePublicURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("CODEBRIDGE_PUBLIC_URL must be an absolute URL without query or fragment")
	}
	if u.Path != "" && u.Path != "/" {
		return errors.New("CODEBRIDGE_PUBLIC_URL must be the public origin, without /mcp or another path")
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" {
		host := u.Hostname()
		if host == "localhost" {
			return nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil
		}
	}
	return errors.New("CODEBRIDGE_PUBLIC_URL must use https (http is allowed only for loopback development)")
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := auditlog.NewRequestID()
		r.Header.Set("X-CodeBridge-Request-ID", id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}

func limitMCP(maxInflight int, maxBodyBytes int64, next http.Handler) http.Handler {
	sem := make(chan struct{}, maxInflight)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > maxBodyBytes {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		}
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
		default:
			w.Header().Set("Retry-After", "1")
			http.Error(w, "too many concurrent MCP requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func optionalBearer(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("request_id=%s method=%s path=%s duration=%s", r.Header.Get("X-CodeBridge-Request-ID"), r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func envInt(k string, def, min, max int) int {
	raw := strings.TrimSpace(os.Getenv(k))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < min || n > max {
		log.Fatalf("%s must be an integer between %d and %d", k, min, max)
	}
	return n
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
