package main

import (
	"crypto/subtle"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	mgr "github.com/edynasty/codebridge/internal/manager"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	addr := flag.String("addr", env("CODEBRIDGE_ADDR", ":8080"), "listen address")
	flag.Parse()

	registry := mgr.NewRegistry()
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "CodeBridge", Version: "0.1.0"}, nil)
	(&mgr.ToolService{Registry: registry}).Register(mcpServer)

	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server { return mcpServer }, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
	})

	mux := http.NewServeMux()
	mux.Handle("/mcp", optionalBearer(os.Getenv("CODEBRIDGE_MCP_TOKEN"), mcpHandler))
	mux.Handle("/agent", &mgr.AgentHandler{Registry: registry, EnrollToken: os.Getenv("CODEBRIDGE_ENROLL_TOKEN")})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"CodeBridge","mcp":"/mcp","agent":"/agent","health":"/healthz"}`))
	})

	s := &http.Server{Addr: *addr, Handler: logRequests(mux), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("CodeBridge manager listening on %s", *addr)
	log.Printf("MCP endpoint: /mcp; agent websocket: /agent")
	if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
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
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
