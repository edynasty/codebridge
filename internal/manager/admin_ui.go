package manager

import (
	"bufio"
	"embed"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/edynasty/codebridge/internal/mcpcallstore"
)

// The admin maintenance UI is a single embedded page served at /admin/ui.
// It authenticates with the same admin bearer token as the JSON API (the
// token is entered once in the browser and kept in sessionStorage only);
// it is never accepted in the URL or a cookie, so nothing leaks into
// history or referer headers.

//go:embed admin.html
var adminIndex string

const adminAuditTailLines = 200

// auditTailer reads the newest lines from the audit log for the UI. The
// manager wires it to the configured audit file; when the audit log goes
// to stdout (no file configured) the tailer stays nil and the UI hides the
// audit panel.
type AuditTailer struct {
	path string
	mu   sync.Mutex
}

func NewAuditTailer(path string) *AuditTailer {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	return &AuditTailer{path: path}
}

func (t *AuditTailer) Tail(n int) []string {
	if t == nil || n <= 0 || n > 1000 {
		n = adminAuditTailLines
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	f, err := os.Open(t.path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var ring []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		ring = append(ring, scanner.Text())
		if len(ring) > n {
			ring = ring[len(ring)-n:]
		}
	}
	return ring
}

// ServeAdminUI renders the admin page. Token auth is enforced client-side
// per API call (the page itself is static and secret-free).
func (h *AdminHandler) ServeAdminUI(w http.ResponseWriter, r *http.Request) {
	if h.AdminToken == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	_, _ = io.WriteString(w, adminIndex)
}

// handleAuditTail returns the newest audit events for the UI. Same bearer
// auth as every admin route; read-only.
func (h *AdminHandler) handleAuditTail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.AuditTail == nil {
		_ = jsonEncode(w, map[string]any{"available": false, "events": []string{}})
		return
	}
	_ = jsonEncode(w, map[string]any{"available": true, "events": h.AuditTail.Tail(adminAuditTailLines)})
}

// handleMCPCalls serves filtered, paged MCP call history from the SQLite
// store. Every parameter is bound, never concatenated.
func (h *AdminHandler) handleMCPCalls(w http.ResponseWriter, r *http.Request) {
	if h.CallStore == nil {
		writeJSONError(w, http.StatusNotFound, "mcp call store is not configured (set CODEBRIDGE_MCP_DB)")
		return
	}
	q := r.URL.Query()
	f := mcpcallstore.Filter{
		Tool:      strings.TrimSpace(q.Get("tool")),
		ActorID:   strings.TrimSpace(q.Get("actor")),
		DeviceID:  strings.TrimSpace(q.Get("device_id")),
		Workspace: strings.TrimSpace(q.Get("workspace")),
		Query:     strings.TrimSpace(q.Get("q")),
	}
	if v := strings.TrimSpace(q.Get("success")); v != "" {
		b := v == "true" || v == "1"
		f.Success = &b
	}
	if v := strings.TrimSpace(q.Get("since")); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Since = t
		}
	}
	if v := strings.TrimSpace(q.Get("until")); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Until = t
		}
	}
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Limit = n
		}
	}
	if v := strings.TrimSpace(q.Get("offset")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Offset = n
		}
	}
	calls, stats, err := h.CallStore.List(f)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = jsonEncode(w, map[string]any{"calls": calls, "stats": stats, "limit": f.Limit, "offset": f.Offset})
}

func jsonEncode(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	return json.NewEncoder(w).Encode(v)
}

var _ = embed.FS{}
