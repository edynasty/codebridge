package manager

import (
	"bufio"
	"embed"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
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

func jsonEncode(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	return json.NewEncoder(w).Encode(v)
}

var _ = embed.FS{}
