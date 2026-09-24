// Package ui serves the client's local configuration interface. It binds to
// loopback by default and guards every mutating call with a per-run random
// token injected into the page, so other local pages cannot drive the API.
package ui

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/edynasty/codebridge/internal/agentops"
	"github.com/edynasty/codebridge/internal/protocol"
	"strings"
	"sync"
	"time"
)

const maxConfigBodyBytes = 256 * 1024

// enrollOnlySentinel marks a SaveConfig call that only injects a new
// one-time enrollment code.
const enrollOnlySentinel = "\x00enroll-only"

// Config is the editable client configuration as shown and written by the UI.
type Config struct {
	ManagerURL          string                           `json:"manager_url"`
	AllowInsecureWS     bool                             `json:"allow_insecure_ws"`
	DeviceID            string                           `json:"device_id"`
	DeviceName          string                           `json:"device_name"`
	AccessMode          string                           `json:"access_mode"`
	Workspaces          map[string]string                `json:"workspaces"`
	WritableWorkspaces  []string                         `json:"writable_workspaces"`
	EnabledTools        []string                         `json:"enabled_tools"`
	DisabledTools       []string                         `json:"disabled_tools"`
	BashAllowlist       []string                         `json:"bash_allowlist"`
	Permissions         []agentops.PermissionRule        `json:"permissions"`
	CustomTools         []protocol.CustomTool            `json:"custom_tools"`
	SubagentProfiles    []agentops.SubagentProfileConfig `json:"subagent_profiles"`
	AllowSensitiveFiles bool                             `json:"allow_sensitive_files"`
	EnableLSP           bool                             `json:"enable_lsp"`
	EnrollmentCode      string                           `json:"enrollment_code,omitempty"`
}

// State is the read-only runtime snapshot for the status panel.
type State struct {
	Connected      bool        `json:"connected"`
	ManagerURL     string      `json:"manager_url"`
	DeviceID       string      `json:"device_id"`
	DeviceName     string      `json:"device_name"`
	Enrolled       bool        `json:"enrolled"`
	CredentialFile string      `json:"credential_file"`
	ConfigFile     string      `json:"config_file"`
	Workspaces     []Workspace `json:"workspaces"`
	StartedAt      time.Time   `json:"started_at"`
	Reconnects     int         `json:"reconnects"`
	Version        string      `json:"version"`
	UIMessage      string      `json:"ui_message,omitempty"`
}

type Workspace struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Writable  bool   `json:"writable"`
	Sensitive bool   `json:"sensitive_allowed"`
}

// Server is the local UI server. Hooks are provided by cmd/client; they run on
// UI goroutines and must be quick and panic-safe.
type Server struct {
	// LoadConfig returns the currently effective configuration.
	LoadConfig func() Config
	// SaveConfig validates and persists a new configuration. Returning a
	// non-empty reload request triggers a reconnect with the new config.
	SaveConfig func(Config) (reload bool, err error)
	// StateSnapshot builds the current status view.
	StateSnapshot func() State
	// Disconnect drops the current manager connection (it reconnects with
	// the same configuration).
	Disconnect func()
	// LogTail returns the last n log lines.
	LogTail func(n int) []string

	token string
	mu    sync.Mutex
	logs  []string
	addr  string
}

// AttachLogRing collects recent log lines for the UI log panel.
type AttachLogRing struct {
	mu    sync.Mutex
	lines []string
	w     io.Writer
}

func NewLogRing(size int, underlying io.Writer) *AttachLogRing {
	return &AttachLogRing{lines: make([]string, 0, size), w: underlying}
}

func (r *AttachLogRing) Write(p []byte) (int, error) {
	r.mu.Lock()
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if line == "" {
			continue
		}
		r.lines = append(r.lines, line)
		if len(r.lines) > cap(r.lines) {
			r.lines = r.lines[1:]
		}
	}
	r.mu.Unlock()
	if r.w != nil {
		return r.w.Write(p)
	}
	return len(p), nil
}

func (r *AttachLogRing) Tail(n int) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n <= 0 || n > len(r.lines) {
		n = len(r.lines)
	}
	out := make([]string, n)
	copy(out, r.lines[len(r.lines)-n:])
	return out
}

func NewServer() *Server {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return &Server{token: hex.EncodeToString(b[:])}
}

// Token exposes the run token so the process can print it (log line).
func (s *Server) Token() string { return s.token }

// Listen starts the HTTP listener on addr and returns the bound address.
// An empty addr means the UI is disabled.
func (s *Server) Listen(addr string) (string, error) {
	if strings.TrimSpace(addr) == "" {
		return "", nil
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("listen %s: %w", addr, err)
	}
	s.addr = listener.Addr().String()
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/enroll", s.handleEnroll)
	mux.HandleFunc("/api/disconnect", s.handleDisconnect)
	mux.HandleFunc("/api/logs", s.handleLogs)
	go func() { _ = http.Serve(listener, mux) }()
	return s.addr, nil
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	page := indexPage
	page = strings.ReplaceAll(page, "__TOKEN__", s.token)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	_, _ = io.WriteString(w, page)
}

func (s *Server) requirePOST(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	if r.Header.Get("X-Codebridge-Token") != s.token {
		http.Error(w, "invalid UI token", http.StatusUnauthorized)
		return false
	}
	return true
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeJSON(w, s.StateSnapshot())
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.writeJSON(w, s.LoadConfig())
	case http.MethodPost:
		if !s.requirePOST(w, r) {
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxConfigBodyBytes+1))
		if err != nil || len(body) > maxConfigBodyBytes {
			http.Error(w, "invalid config body", http.StatusBadRequest)
			return
		}
		var in Config
		if err := json.Unmarshal(body, &in); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		reload, err := s.SaveConfig(in)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.writeJSON(w, map[string]any{"saved": true, "reconnecting": reload})
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if !s.requirePOST(w, r) {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	var in struct {
		EnrollmentCode string `json:"enrollment_code"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(in.EnrollmentCode) == "" {
		http.Error(w, "enrollment_code is required", http.StatusBadRequest)
		return
	}
	// ManagerURL carries a sentinel so SaveConfig can tell "set the enroll
	// code only" apart from a full configuration update.
	if _, err := s.SaveConfig(Config{EnrollmentCode: strings.TrimSpace(in.EnrollmentCode), ManagerURL: enrollOnlySentinel}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.writeJSON(w, map[string]any{"saved": true, "reconnecting": true})
}

func (s *Server) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if !s.requirePOST(w, r) {
		return
	}
	s.Disconnect()
	s.writeJSON(w, map[string]any{"disconnected": true})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeJSON(w, map[string]any{"lines": s.LogTail(200)})
}

func (s *Server) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}
