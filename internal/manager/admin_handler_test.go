package manager

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/edynasty/codebridge/internal/authstore"
)

func newTestAdminHandler(t *testing.T) (*AdminHandler, *authstore.Store, *httptest.Server) {
	t.Helper()
	store, err := authstore.Open("")
	if err != nil {
		t.Fatal(err)
	}
	handler := &AdminHandler{Auth: store, Registry: NewRegistry(2), AdminToken: "test-admin-token"}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return handler, store, server
}

func adminRequest(t *testing.T, server *httptest.Server, method, path, body string) *http.Response {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequest(method, server.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test-admin-token")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestAdminCreateEnrollmentTargetsAccount(t *testing.T) {
	_, store, server := newTestAdminHandler(t)

	resp := adminRequest(t, server, http.MethodPost, "/admin/enrollments", `{"account_id":"team-a"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enrollment creation failed: %d", resp.StatusCode)
	}
	var created struct {
		EnrollmentCode string `json:"enrollment_code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnrollDevice(created.EnrollmentCode, "dev-a", "Dev A"); err != nil {
		t.Fatalf("enroll with targeted code: %v", err)
	}
	if account, ok := store.DeviceAccount("dev-a"); !ok || account != "team-a" {
		t.Fatalf("device not enrolled into team-a: %q %v", account, ok)
	}
}

func TestAdminCreateEnrollmentDefaultsAccount(t *testing.T) {
	_, store, server := newTestAdminHandler(t)

	resp := adminRequest(t, server, http.MethodPost, "/admin/enrollments", `{}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enrollment creation failed: %d", resp.StatusCode)
	}
	var created struct {
		EnrollmentCode string `json:"enrollment_code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnrollDevice(created.EnrollmentCode, "dev-default", "Dev"); err != nil {
		t.Fatalf("enroll with default code: %v", err)
	}
	if account, ok := store.DeviceAccount("dev-default"); !ok || account != authstore.DefaultAccount {
		t.Fatalf("device not enrolled into default account: %q %v", account, ok)
	}
}

func TestAdminCreateEnrollmentRejectsOversizedAccount(t *testing.T) {
	_, _, server := newTestAdminHandler(t)
	oversized := `{"account_id":"` + strings.Repeat("x", 129) + `"}`
	resp := adminRequest(t, server, http.MethodPost, "/admin/enrollments", oversized)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized account accepted: %d", resp.StatusCode)
	}
}

func TestAdminListDevicesIncludesAccount(t *testing.T) {
	_, store, server := newTestAdminHandler(t)
	code, _, err := store.CreateEnrollment(time.Minute, "team-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnrollDevice(code, "dev-a", "Dev A"); err != nil {
		t.Fatal(err)
	}

	resp := adminRequest(t, server, http.MethodGet, "/admin/devices", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list devices failed: %d", resp.StatusCode)
	}
	var devices []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0]["account_id"] != "team-a" {
		t.Fatalf("account_id missing from device list: %#v", devices)
	}
}

func TestAdminRejectsInvalidBearer(t *testing.T) {
	_, _, server := newTestAdminHandler(t)
	req, err := http.NewRequest(http.MethodGet, server.URL+"/admin/devices", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong bearer accepted: %d", resp.StatusCode)
	}
}
