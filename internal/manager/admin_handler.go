package manager

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/edynasty/codebridge/internal/auditlog"
	"github.com/edynasty/codebridge/internal/authstore"
)

type AdminHandler struct {
	Auth       *authstore.Store
	Registry   *Registry
	AdminToken string
	Audit      *auditlog.Logger
}

type createEnrollmentRequest struct {
	TTLSeconds int `json:"ttl_seconds,omitempty"`
}

func (h *AdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.AdminToken == "" {
		http.NotFound(w, r)
		return
	}
	if !bearerMatches(r.Header.Get("Authorization"), h.AdminToken) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	path := strings.TrimPrefix(r.URL.Path, "/admin")
	switch {
	case path == "/enrollments" && r.Method == http.MethodPost:
		h.createEnrollment(w, r)
	case path == "/devices" && r.Method == http.MethodGet:
		h.listDevices(w)
	case strings.HasPrefix(path, "/devices/"):
		h.deviceAction(w, r, strings.TrimPrefix(path, "/devices/"))
	default:
		http.NotFound(w, r)
	}
}

func (h *AdminHandler) createEnrollment(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	requestID := r.Header.Get(requestIDHeader)
	var in createEnrollmentRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&in)
	}
	ttl := 10 * time.Minute
	if in.TTLSeconds > 0 {
		ttl = time.Duration(in.TTLSeconds) * time.Second
	}
	code, expires, err := h.Auth.CreateEnrollment(ttl)
	if err != nil {
		_ = h.Audit.Log(auditlog.Event{Event: "admin.enrollment.create", RequestID: requestID, ActorID: "admin", Success: auditlog.Bool(false), DurationMS: time.Since(start).Milliseconds(), ErrorKind: "store_error"})
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"enrollment_code": code,
		"expires_at":      expires,
		"expires_in":      int(time.Until(expires).Seconds()),
	})
	_ = h.Audit.Log(auditlog.Event{Event: "admin.enrollment.create", RequestID: requestID, ActorID: "admin", Success: auditlog.Bool(true), DurationMS: time.Since(start).Milliseconds()})
}

func (h *AdminHandler) listDevices(w http.ResponseWriter) {
	online := map[string]bool{}
	if h.Registry != nil {
		for _, d := range h.Registry.List() {
			online[d.ID] = true
		}
	}
	devices := h.Auth.ListDevices()
	out := make([]map[string]any, 0, len(devices))
	for _, d := range devices {
		out = append(out, map[string]any{
			"id":         d.ID,
			"name":       d.Name,
			"created_at": d.CreatedAt,
			"updated_at": d.UpdatedAt,
			"online":     online[d.ID],
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (h *AdminHandler) deviceAction(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	deviceID := parts[0]
	if len(parts) == 1 && r.Method == http.MethodDelete {
		start := time.Now()
		requestID := r.Header.Get(requestIDHeader)
		if err := h.Auth.RevokeDevice(deviceID); err != nil {
			_ = h.Audit.Log(auditlog.Event{Event: "admin.device.revoke", RequestID: requestID, ActorID: "admin", DeviceID: deviceID, Success: auditlog.Bool(false), DurationMS: time.Since(start).Milliseconds(), ErrorKind: "not_found"})
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		if h.Registry != nil {
			h.Registry.Disconnect(deviceID)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"revoked": true, "device_id": deviceID})
		_ = h.Audit.Log(auditlog.Event{Event: "admin.device.revoke", RequestID: requestID, ActorID: "admin", DeviceID: deviceID, Success: auditlog.Bool(true), DurationMS: time.Since(start).Milliseconds()})
		return
	}
	if len(parts) == 2 && parts[1] == "rotate" && r.Method == http.MethodPost {
		start := time.Now()
		requestID := r.Header.Get(requestIDHeader)
		credential, err := h.Auth.RotateDevice(deviceID)
		if err != nil {
			_ = h.Audit.Log(auditlog.Event{Event: "admin.device.rotate", RequestID: requestID, ActorID: "admin", DeviceID: deviceID, Success: auditlog.Bool(false), DurationMS: time.Since(start).Milliseconds(), ErrorKind: "not_found"})
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		if h.Registry != nil {
			h.Registry.Disconnect(deviceID)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_id":         deviceID,
			"device_credential": credential,
			"note":              "This credential is shown once. Update the client credential file before its next reconnect.",
		})
		_ = h.Audit.Log(auditlog.Event{Event: "admin.device.rotate", RequestID: requestID, ActorID: "admin", DeviceID: deviceID, Success: auditlog.Bool(true), DurationMS: time.Since(start).Milliseconds()})
		return
	}
	http.NotFound(w, r)
}

func bearerMatches(header, want string) bool {
	if want == "" {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg, "status": strconv.Itoa(status)})
}
